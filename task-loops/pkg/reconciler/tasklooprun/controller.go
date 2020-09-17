/*
Copyright 2020 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tasklooprun

import (
	"context"

	"github.com/tektoncd/experimental/task-loops/pkg/apis/taskloop"
	taskloopv1alpha1 "github.com/tektoncd/experimental/task-loops/pkg/apis/taskloop/v1alpha1"
	taskloopinformer "github.com/tektoncd/experimental/task-loops/pkg/client/injection/informers/taskloop/v1alpha1/taskloop"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	pipelineclient "github.com/tektoncd/pipeline/pkg/client/injection/client"
	runinformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1alpha1/run"
	taskruninformer "github.com/tektoncd/pipeline/pkg/client/injection/informers/pipeline/v1beta1/taskrun"
	runreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1alpha1/run"
	listersalpha "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1alpha1"
	pipelinecontroller "github.com/tektoncd/pipeline/pkg/controller"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// NewController instantiates a new controller.Impl from knative.dev/pkg/controller
func NewController(namespace string) func(context.Context, configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {

		logger := logging.FromContext(ctx)
		pipelineclientset := pipelineclient.Get(ctx)
		runInformer := runinformer.Get(ctx)
		taskLoopInformer := taskloopinformer.Get(ctx)
		taskRunInformer := taskruninformer.Get(ctx)

		c := &Reconciler{
			PipelineClientSet: pipelineclientset,
			runLister:         runInformer.Lister(),
			taskLoopLister:    taskLoopInformer.Lister(),
			taskRunLister:     taskRunInformer.Lister(),
		}

		impl := runreconciler.NewImpl(ctx, c, func(impl *controller.Impl) controller.Options {
			return controller.Options{
				AgentName: "Run (task loop)", // TODO: Fix name
			}
		})

		logger.Info("Setting up event handlers")

		// Add event handler for Runs
		runInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: pipelinecontroller.FilterRunRef(taskloopv1alpha1.SchemeGroupVersion.String(), taskloop.TaskLoopControllerName),
			Handler:    controller.HandleAll(impl.Enqueue),
		})

		// Add event handler for TaskRuns controlled by Run
		taskRunInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
			FilterFunc: filterOwnerRunRef(logger, runInformer.Lister(), taskloopv1alpha1.SchemeGroupVersion.String(), taskloop.TaskLoopControllerName),
			Handler:    controller.HandleAll(impl.EnqueueControllerOf),
		})

		return impl
	}
}

// TODO: If this works, move it to filter.go alongside the other one?
func filterOwnerRunRef(logger *zap.SugaredLogger, runLister listersalpha.RunLister, apiVersion, kind string) func(interface{}) bool {
	return func(obj interface{}) bool {
		object, ok := obj.(metav1.Object)
		if !ok {
			return false
		}
		owner := metav1.GetControllerOf(object)
		if owner == nil {
			return false
		}
		if owner.APIVersion != v1alpha1.SchemeGroupVersion.String() || owner.Kind != pipeline.PipelineRunControllerName {
			// Not owned by a Run
			return false
		}
		run, err := runLister.Runs(object.GetNamespace()).Get(owner.Name)
		if err != nil {
			logger.Infof("Failed to get Run %s which owns %s", owner.Name, object.GetName())
			return false
		}
		if run.Spec.Ref == nil {
			// These are invalid, but just in case they get created somehow, don't panic.
			return false
		}
		return run.Spec.Ref.APIVersion == apiVersion && run.Spec.Ref.Kind == v1alpha1.TaskKind(kind)
	}
}

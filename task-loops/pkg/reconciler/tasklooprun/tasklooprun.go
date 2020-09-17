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
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/tektoncd/experimental/task-loops/pkg/apis/taskloop"
	taskloopv1alpha1 "github.com/tektoncd/experimental/task-loops/pkg/apis/taskloop/v1alpha1"
	listerstaskloop "github.com/tektoncd/experimental/task-loops/pkg/client/listers/taskloop/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	clientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	runreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1alpha1/run"
	listersalpha "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1alpha1"
	listers "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/reconciler/events"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/logging"
	pkgreconciler "knative.dev/pkg/reconciler"
)

const (
	// taskLoopLabelKey is used as the label identifier for a TaskLoop
	taskLoopLabelKey = "/taskLoop"

	// taskLoopRunLabelKey is used as the label identifier for a TaskLoopRun
	taskLoopRunLabelKey = "/taskLoopRun"

	// taskLoopIterationLabelKey is used as the label identifier for the iteration number
	taskLoopIterationLabelKey = "/taskLoopIteration"
)

// Reconciler implements controller.Reconciler for Configuration resources.
type Reconciler struct {
	PipelineClientSet clientset.Interface
	runLister         listersalpha.RunLister
	taskLoopLister    listerstaskloop.TaskLoopLister
	taskRunLister     listers.TaskRunLister
}

var (
	// Check that our Reconciler implements runreconciler.Interface
	_ runreconciler.Interface = (*Reconciler)(nil)
)

// ReconcileKind compares the actual state with the desired, and attempts to converge the two.
// It then updates the Status block of the Run resource with the current status of the resource.
func (c *Reconciler) ReconcileKind(ctx context.Context, run *v1alpha1.Run) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof("Reconciling Run %s/%s at %v", run.Namespace, run.Name, time.Now())

	// TODO: Add bullet proofing check for Run referencing a TaskLoop

	// If the Run has not started, initialize the Condition and set the start time.
	if !run.HasStarted() {
		logger.Infof("Starting new Run %s/%s", run.Namespace, run.Name)
		run.Status.InitializeConditions()
		// In case node time was not synchronized, when controller has been scheduled to other nodes.
		if run.Status.StartTime.Sub(run.CreationTimestamp.Time) < 0 {
			logger.Warnf("Run %s createTimestamp %s is after the Run started %s", run.Name, run.CreationTimestamp, run.Status.StartTime)
			run.Status.StartTime = &run.CreationTimestamp
		}
		// Emit events. During the first reconcile the status of the Run may change twice
		// from not Started to Started and then to Running, so we need to sent the event here
		// and at the end of 'Reconcile' again.
		// We also want to send the "Started" event as soon as possible for anyone who may be waiting
		// on the event to perform user facing initialisations, such has reset a CI check status
		afterCondition := run.Status.GetCondition(apis.ConditionSucceeded)
		events.Emit(ctx, nil, afterCondition, run)
	}

	if run.IsDone() {
		logger.Infof("Run %s/%s is done", run.Namespace, run.Name)
		// TODO: metrics (metrics.DurationAndCount) -- this might not be right place (double counting)
		return nil
	}

	// Store the condition before reconcile
	beforeCondition := run.Status.GetCondition(apis.ConditionSucceeded)

	// Reconcile this copy of the TaskLoopRun
	err := c.reconcile(ctx, run)
	if err != nil {
		logger.Errorf("Reconcile error: %v", err.Error())
	}

	if err := c.updateLabelsAndAnnotations(run); err != nil {
		logger.Warn("Failed to update Run labels/annotations", zap.Error(err))
		// TODO: Include in multierror
	}

	afterCondition := run.Status.GetCondition(apis.ConditionSucceeded)
	events.Emit(ctx, beforeCondition, afterCondition, run)

	// TODO: Need to implement the multierror stuff?
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, run *v1alpha1.Run) error {

	// Get the TaskLoop referenced by the Run
	taskLoopMeta, _, err := c.getTaskLoop(run)
	if err != nil {
		return nil
	}

	// Store the fetched TaskLoopSpec on the TaskLoopRun for auditing
	// TODO: FIXME - storing stuff in Run is different
	// storeTaskLoopSpec(tlr, taskLoopSpec)

	// Propagate labels and annotations from TaskLoop to Run.
	propagateTaskLoopLabelsAndAnnotations(run, taskLoopMeta)

	return nil
}

func (c *Reconciler) getTaskLoop(run *v1alpha1.Run) (*metav1.ObjectMeta, *taskloopv1alpha1.TaskLoopSpec, error) {
	taskLoopMeta := metav1.ObjectMeta{}
	taskLoopSpec := taskloopv1alpha1.TaskLoopSpec{}
	if run.Spec.Ref != nil && run.Spec.Ref.Name != "" {
		tl, err := c.taskLoopLister.TaskLoops(run.Namespace).Get(run.Spec.Ref.Name)
		if err != nil {
			run.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  taskloopv1alpha1.TaskLoopRunReasonCouldntGetTaskLoop.String(),
				Message: fmt.Sprintf("Error retrieving TaskLoop for Run %s/%s: %s", run.Namespace, run.Name, err),
			})
			return nil, nil, fmt.Errorf("Error retrieving TaskLoop for Run %s: %w", fmt.Sprintf("%s/%s", run.Namespace, run.Name), err)
		}
		taskLoopMeta = tl.ObjectMeta
		taskLoopSpec = tl.Spec
	} else {
		// Run does not require name but for TaskLoop it does.
		run.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  taskloopv1alpha1.TaskLoopRunReasonCouldntGetTaskLoop.String(),
			Message: fmt.Sprintf("Missing taskLoopRef for TaskLoopRun %s/%s", run.Namespace, run.Name),
		})
		return nil, nil, fmt.Errorf("Missing taskLoopRef for TaskLoopRun %s", fmt.Sprintf("%s/%s", run.Namespace, run.Name))
	}
	return &taskLoopMeta, &taskLoopSpec, nil
}

func (c *Reconciler) updateLabelsAndAnnotations(run *v1alpha1.Run) error {
	newRun, err := c.runLister.Runs(run.Namespace).Get(run.Name)
	if err != nil {
		return fmt.Errorf("error getting Run %s when updating labels/annotations: %w", run.Name, err)
	}
	if !reflect.DeepEqual(run.ObjectMeta.Labels, newRun.ObjectMeta.Labels) || !reflect.DeepEqual(run.ObjectMeta.Annotations, newRun.ObjectMeta.Annotations) {
		mergePatch := map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels":      run.ObjectMeta.Labels,
				"annotations": run.ObjectMeta.Annotations,
			},
		}
		patch, err := json.Marshal(mergePatch)
		if err != nil {
			return err
		}
		_, err = c.PipelineClientSet.TektonV1alpha1().Runs(run.Namespace).Patch(run.Name, types.MergePatchType, patch)
		return err
	}
	return nil
}

func propagateTaskLoopLabelsAndAnnotations(run *v1alpha1.Run, taskLoopMeta *metav1.ObjectMeta) {
	// Propagate labels from TaskLoop to Run.
	if run.ObjectMeta.Labels == nil {
		run.ObjectMeta.Labels = make(map[string]string, len(taskLoopMeta.Labels)+1)
	}
	for key, value := range taskLoopMeta.Labels {
		run.ObjectMeta.Labels[key] = value
	}
	run.ObjectMeta.Labels[taskloop.GroupName+taskLoopLabelKey] = taskLoopMeta.Name

	// Propagate annotations from TaskLoop to TaskLoopRun.
	if run.ObjectMeta.Annotations == nil {
		run.ObjectMeta.Annotations = make(map[string]string, len(taskLoopMeta.Annotations))
	}
	for key, value := range taskLoopMeta.Annotations {
		run.ObjectMeta.Annotations[key] = value
	}
}

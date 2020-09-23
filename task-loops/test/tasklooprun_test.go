// +build e2e

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

package test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tektoncd/experimental/task-loops/pkg/apis/taskloop"
	taskloopv1alpha1 "github.com/tektoncd/experimental/task-loops/pkg/apis/taskloop/v1alpha1"
	"github.com/tektoncd/experimental/task-loops/pkg/client/clientset/versioned"
	resourceversioned "github.com/tektoncd/experimental/task-loops/pkg/client/clientset/versioned/typed/taskloop/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/pod"
	tektontest "github.com/tektoncd/pipeline/test"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	knativetest "knative.dev/pkg/test"
)

var (
	runTimeout              = 10 * time.Minute
	startedEventMessage     = "" // Run started event has no message
	ignoreReleaseAnnotation = func(k string, v string) bool {
		return k == pod.ReleaseAnnotation
	}
)

var aTask = &v1beta1.Task{
	ObjectMeta: metav1.ObjectMeta{Name: "a-task"},
	Spec: v1beta1.TaskSpec{
		Params: []v1beta1.ParamSpec{{
			Name: "current-item",
			Type: v1beta1.ParamTypeString,
		}, {
			Name: "fail-on-item",
			Type: v1beta1.ParamTypeString,
		}},
		Steps: []v1beta1.Step{{
			Container: corev1.Container{
				Name:    "passfail",
				Image:   "ubuntu",
				Command: []string{"/bin/bash"},
				Args:    []string{"-c", "[[ $(params.current-item) != $(params.fail-on-item) ]]"},
			},
		}},
	},
}

var aTaskLoop = &taskloopv1alpha1.TaskLoop{
	ObjectMeta: metav1.ObjectMeta{Name: "a-taskloop"},
	Spec: taskloopv1alpha1.TaskLoopSpec{
		TaskRef:      &v1beta1.TaskRef{Name: "a-task"},
		IterateParam: "current-item",
	},
}

var runTaskLoopSuccess = &v1alpha1.Run{
	ObjectMeta: metav1.ObjectMeta{
		Name: "run-taskloop",
		Labels: map[string]string{
			"myTestLabel": "myTestLabelValue",
		},
		Annotations: map[string]string{
			"myTestAnnotation": "myTestAnnotationValue",
		},
	},
	Spec: v1alpha1.RunSpec{
		Params: []v1beta1.Param{{
			Name:  "current-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeArray, ArrayVal: []string{"item1", "item2"}},
		}, {
			Name:  "fail-on-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "dontfailonanyitem"},
		}},
		Ref: &v1alpha1.TaskRef{
			APIVersion: taskloopv1alpha1.SchemeGroupVersion.String(),
			Kind:       taskloop.TaskLoopControllerName,
			Name:       "a-taskloop",
		},
	},
}

var runTaskLoopFailure = &v1alpha1.Run{
	ObjectMeta: metav1.ObjectMeta{
		Name: "run-taskloop",
		Labels: map[string]string{
			"myTestLabel": "myTestLabelValue",
		},
		Annotations: map[string]string{
			"myTestAnnotation": "myTestAnnotationValue",
		},
	},
	Spec: v1alpha1.RunSpec{
		Params: []v1beta1.Param{{
			Name:  "current-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeArray, ArrayVal: []string{"item1", "item2"}},
		}, {
			Name:  "fail-on-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "item1"},
		}},
		Ref: &v1alpha1.TaskRef{
			APIVersion: taskloopv1alpha1.SchemeGroupVersion.String(),
			Kind:       taskloop.TaskLoopControllerName,
			Name:       "a-taskloop",
		},
	},
}

var expectedTaskRunIteration1Success = &v1beta1.TaskRun{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "run-taskloop-00001-", // does not include random suffix
		Namespace: "foo",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by":        "tekton-pipelines",
			"tekton.dev/run":                      "run-taskloop",
			"tekton.dev/task":                     "a-task",
			"custom.tekton.dev/taskLoop":          "a-taskloop",
			"custom.tekton.dev/taskLoopIteration": "1",
			"myTestLabel":                         "myTestLabelValue",
		},
		Annotations: map[string]string{
			"myTestAnnotation": "myTestAnnotationValue",
		},
	},
	Spec: v1beta1.TaskRunSpec{
		TaskRef: &v1beta1.TaskRef{Name: "a-task", Kind: "Task"},
		Timeout: &metav1.Duration{Duration: 1 * time.Hour}, // default TaskRun timeout
		Params: []v1beta1.Param{{
			Name:  "current-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "item1"},
		}, {
			Name:  "fail-on-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "dontfailonanyitem"},
		}},
	},
	Status: v1beta1.TaskRunStatus{
		Status: duckv1beta1.Status{
			Conditions: []apis.Condition{{
				Type:   apis.ConditionSucceeded,
				Status: corev1.ConditionTrue,
				Reason: v1beta1.TaskRunReasonSuccessful.String(),
			}},
		},
	},
}

var expectedTaskRunIteration2Success = &v1beta1.TaskRun{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "run-taskloop-00002-", // does not include random suffix
		Namespace: "foo",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by":        "tekton-pipelines",
			"tekton.dev/run":                      "run-taskloop",
			"tekton.dev/task":                     "a-task",
			"custom.tekton.dev/taskLoop":          "a-taskloop",
			"custom.tekton.dev/taskLoopIteration": "2",
			"myTestLabel":                         "myTestLabelValue",
		},
		Annotations: map[string]string{
			"myTestAnnotation": "myTestAnnotationValue",
		},
	},
	Spec: v1beta1.TaskRunSpec{
		TaskRef: &v1beta1.TaskRef{Name: "a-task", Kind: "Task"},
		Timeout: &metav1.Duration{Duration: 1 * time.Hour}, // default TaskRun timeout
		Params: []v1beta1.Param{{
			Name:  "current-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "item2"},
		}, {
			Name:  "fail-on-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "dontfailonanyitem"},
		}},
	},
	Status: v1beta1.TaskRunStatus{
		Status: duckv1beta1.Status{
			Conditions: []apis.Condition{{
				Type:   apis.ConditionSucceeded,
				Status: corev1.ConditionTrue,
				Reason: v1beta1.TaskRunReasonSuccessful.String(),
			}},
		},
	},
}

var expectedTaskRunIteration1Failure = &v1beta1.TaskRun{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "run-taskloop-00001-", // does not include random suffix
		Namespace: "foo",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by":        "tekton-pipelines",
			"tekton.dev/run":                      "run-taskloop",
			"tekton.dev/task":                     "a-task",
			"custom.tekton.dev/taskLoop":          "a-taskloop",
			"custom.tekton.dev/taskLoopIteration": "1",
			"myTestLabel":                         "myTestLabelValue",
		},
		Annotations: map[string]string{
			"myTestAnnotation": "myTestAnnotationValue",
		},
	},
	Spec: v1beta1.TaskRunSpec{
		TaskRef: &v1beta1.TaskRef{Name: "a-task", Kind: "Task"},
		Timeout: &metav1.Duration{Duration: 1 * time.Hour}, // default TaskRun timeout
		Params: []v1beta1.Param{{
			Name:  "current-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "item1"},
		}, {
			Name:  "fail-on-item",
			Value: v1beta1.ArrayOrString{Type: v1beta1.ParamTypeString, StringVal: "item1"},
		}},
	},
	Status: v1beta1.TaskRunStatus{
		Status: duckv1beta1.Status{
			Conditions: []apis.Condition{{
				Type:   apis.ConditionSucceeded,
				Status: corev1.ConditionFalse,
				Reason: v1beta1.TaskRunReasonFailed.String(),
			}},
		},
	},
}

func TestTaskLoopRun(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		// The following set of fields describe the resources to create.
		task     *v1beta1.Task
		taskloop *taskloopv1alpha1.TaskLoop
		run      *v1alpha1.Run
		// The following set of fields describe the expected outcome.
		expectedStatus   corev1.ConditionStatus
		expectedReason   taskloopv1alpha1.TaskLoopRunReason
		expectedTaskruns []*v1beta1.TaskRun
		expectedEvents   []string
	}{{
		name:             "successful TaskLoop",
		task:             aTask,
		taskloop:         aTaskLoop,
		run:              runTaskLoopSuccess,
		expectedStatus:   corev1.ConditionTrue,
		expectedReason:   taskloopv1alpha1.TaskLoopRunReasonSucceeded,
		expectedTaskruns: []*v1beta1.TaskRun{expectedTaskRunIteration1Success, expectedTaskRunIteration2Success},
		expectedEvents:   []string{startedEventMessage, "Iterations completed: 0", "Iterations completed: 1", "All TaskRuns completed successfully"},
	}, {
		name:             "failed TaskLoop",
		task:             aTask,
		taskloop:         aTaskLoop,
		run:              runTaskLoopFailure,
		expectedStatus:   corev1.ConditionFalse,
		expectedReason:   taskloopv1alpha1.TaskLoopRunReasonFailed,
		expectedTaskruns: []*v1beta1.TaskRun{expectedTaskRunIteration1Failure},
		expectedEvents:   []string{startedEventMessage, "Iterations completed: 0", "TaskRun run-taskloop-00001-.* has failed"},
	}}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tc := tc // Copy current tc to local variable due to test parallelization
			t.Parallel()
			c, namespace := tektontest.Setup(t)
			taskLoopClient := getTaskLoopClient(t, namespace)

			knativetest.CleanupOnInterrupt(func() { tektontest.TearDown(t, c, namespace) }, t.Logf)
			defer tektontest.TearDown(t, c, namespace)

			if tc.task != nil {
				task := tc.task.DeepCopy()
				task.Namespace = namespace
				if _, err := c.TaskClient.Create(task); err != nil {
					t.Fatalf("Failed to create Task `%s`: %s", task.Name, err)
				}
			}

			if tc.taskloop != nil {
				taskloop := tc.taskloop.DeepCopy()
				taskloop.Namespace = namespace
				if _, err := taskLoopClient.Create(taskloop); err != nil {
					t.Fatalf("Failed to create TaskLoop `%s`: %s", tc.taskloop.Name, err)
				}
			}

			run := tc.run.DeepCopy()
			run.Namespace = namespace
			run, err := c.RunClient.Create(tc.run)
			if err != nil {
				t.Fatalf("Failed to create Run `%s`: %s", run.Name, err)
			}

			t.Logf("Waiting for Run %s in namespace %s to complete", run.Name, run.Namespace)
			var inState tektontest.ConditionAccessorFn
			var desc string
			if tc.expectedStatus == corev1.ConditionTrue {
				inState = tektontest.Succeed(run.Name)
				desc = "RunSuccess"
			} else {
				inState = tektontest.FailedWithReason(tc.expectedReason.String(), run.Name)
				desc = "RunFailed"
			}
			if err := tektontest.WaitForRunState(c, run.Name, runTimeout, inState, desc); err != nil {
				t.Fatalf("Error waiting for Run %s/%s to finish: %s", run.Namespace, run.Name, err)
			}

			run, err = c.RunClient.Get(run.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Couldn't get expected Run %s/%s: %s", run.Namespace, run.Name, err)
			}

			t.Logf("Making sure the expected TaskRuns were created")
			actualTaskrunList, err := c.TaskRunClient.List(metav1.ListOptions{LabelSelector: fmt.Sprintf("tekton.dev/run=%s", run.Name)})
			if err != nil {
				t.Fatalf("Error listing TaskRuns for Run %s/%s: %s", run.Namespace, run.Name, err)
			}

			if len(tc.expectedTaskruns) != len(actualTaskrunList.Items) {
				t.Errorf("Expected %d TaskRuns for Run %s/%s but found %d",
					len(tc.expectedTaskruns), run.Namespace, run.Name, len(actualTaskrunList.Items))
			}

			// Check TaskRun status in the Run's status.
			status := &taskloopv1alpha1.TaskLoopRunStatus{}
			if err := run.Status.DecodeExtraFields(status); err != nil {
				t.Errorf("DecodeExtraFields error: %v", err.Error())
			}
			for i, expectedTaskrun := range tc.expectedTaskruns {
				var actualTaskrun v1beta1.TaskRun
				found := false
				for _, actualTaskrun = range actualTaskrunList.Items {
					if strings.HasPrefix(actualTaskrun.Name, expectedTaskrun.Name) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected TaskRun with prefix %s for Run %s/%s not found",
						expectedTaskrun.Name, run.Namespace, run.Name)
					continue
				}
				if d := cmp.Diff(expectedTaskrun.Spec, actualTaskrun.Spec); d != "" {
					t.Errorf("TaskRun %s spec does not match expected spec. Diff %s", actualTaskrun.Name, diff.PrintWantGot(d))
				}
				if d := cmp.Diff(expectedTaskrun.ObjectMeta.Annotations, actualTaskrun.ObjectMeta.Annotations,
					cmpopts.IgnoreMapEntries(ignoreReleaseAnnotation)); d != "" {
					t.Errorf("TaskRun %s does not have expected annotations. Diff %s", actualTaskrun.Name, diff.PrintWantGot(d))
				}
				if d := cmp.Diff(expectedTaskrun.ObjectMeta.Labels, actualTaskrun.ObjectMeta.Labels); d != "" {
					t.Errorf("TaskRun %s does not have expected labels. Diff %s", actualTaskrun.Name, diff.PrintWantGot(d))
				}
				if d := cmp.Diff(expectedTaskrun.Status.Status.Conditions, actualTaskrun.Status.Status.Conditions,
					cmpopts.IgnoreTypes(apis.Condition{}.Message, apis.Condition{}.LastTransitionTime)); d != "" {
					t.Errorf("TaskRun %s does not have expected status condition. Diff %s", actualTaskrun.Name, diff.PrintWantGot(d))
				}

				taskRunStatusInTaskLoopRun, exists := status.TaskRuns[actualTaskrun.Name]
				if !exists {
					t.Errorf("Run status does not include TaskRun status for TaskRun %s", actualTaskrun.Name)
				} else {
					if d := cmp.Diff(expectedTaskrun.Status.Status.Conditions, taskRunStatusInTaskLoopRun.Status.Status.Conditions,
						cmpopts.IgnoreTypes(apis.Condition{}.Message, apis.Condition{}.LastTransitionTime)); d != "" {
						t.Errorf("Run status for TaskRun %s does not have expected status condition. Diff %s",
							actualTaskrun.Name, diff.PrintWantGot(d))
					}
					if i+1 != taskRunStatusInTaskLoopRun.Iteration {
						t.Errorf("Run status for TaskRun %s has iteration number %d instead of %d",
							actualTaskrun.Name, taskRunStatusInTaskLoopRun.Iteration, i+1)
					}
				}
			}

			t.Logf("Checking events that were created from Run")
			matchKinds := map[string][]string{"Run": {run.Name}}
			events, err := collectMatchingEvents(c.KubeClient, namespace, matchKinds)
			if err != nil {
				t.Fatalf("Failed to collect matching events: %q", err)
			}
			for e, expectedEvent := range tc.expectedEvents {
				if e >= len(events) {
					t.Errorf("Expected %d events but got %d", len(tc.expectedEvents), len(events))
					break
				}
				if matched, _ := regexp.MatchString(expectedEvent, events[e].Message); !matched {
					t.Errorf("Expected event %q but got %q", expectedEvent, events[e].Message)
				}
			}
		})
	}
}

func getTaskLoopClient(t *testing.T, namespace string) resourceversioned.TaskLoopInterface {
	configPath := knativetest.Flags.Kubeconfig
	clusterName := knativetest.Flags.Cluster
	cfg, err := knativetest.BuildClientConfig(configPath, clusterName)
	if err != nil {
		t.Fatalf("failed to create configuration obj from %s for cluster %s: %s", configPath, clusterName, err)
	}
	cs, err := versioned.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create taskloop clientset from config file at %s: %s", configPath, err)
	}
	return cs.CustomV1alpha1().TaskLoops(namespace)
}

// collectMatchingEvents collects a list of events under 5 seconds that match certain objects by kind and name.
// This is copied from pipelinerun_test and modified to drop the reason parameter.
func collectMatchingEvents(kubeClient *knativetest.KubeClient, namespace string, kinds map[string][]string) ([]*corev1.Event, error) {
	var events []*corev1.Event

	watchEvents, err := kubeClient.Kube.CoreV1().Events(namespace).Watch(metav1.ListOptions{})
	// close watchEvents channel
	defer watchEvents.Stop()
	if err != nil {
		return events, err
	}

	// create timer to not wait for events longer than 5 seconds
	timer := time.NewTimer(5 * time.Second)

	for {
		select {
		case wevent := <-watchEvents.ResultChan():
			event := wevent.Object.(*corev1.Event)
			if val, ok := kinds[event.InvolvedObject.Kind]; ok {
				for _, expectedName := range val {
					if event.InvolvedObject.Name == expectedName {
						events = append(events, event)
					}
				}
			}
		case <-timer.C:
			return events, nil
		}
	}
}

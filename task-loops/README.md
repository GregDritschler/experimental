# Task Loop Extension

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/kubernetes/experimental/blob/master/LICENSE)

The Task Loop Extension for Tekton allows users to run a Task in a loop with varying parameter values.
This functionality is provided by a controller that implements the [Tekton Custom Task interface](https://github.com/tektoncd/pipeline/blob/master/docs/runs.md).

This is an **_experimental feature_**.  The purpose is to explore potential use cases for looping in Tekton and how they may be achieved.

## Install

## Usage

Two resources are required to run a Task in a loop:

* A `TaskLoop` defines the [Task](https://github.com/tektoncd/pipeline/blob/master/docs/tasks.md) to run and how to iterate it.
* A `Run` executes the TaskLoop and provides parameters to pass to the Task.

### Configuring a `TaskLoop`

A `TaskLoop` definition supports the following fields:

- Required:
  - [`apiVersion`][kubernetes-overview] - Specifies the API version, `custom.tekton.dev/v1alpha1`.
  - [`kind`][kubernetes-overview] - Identifies this resource object as a `TaskLoop` object.
  - [`metadata`][kubernetes-overview] - Specifies the metadata that uniquely identifies the `TaskLoop`, such as a `name`.
  - [`spec`][kubernetes-overview] - Specifies the configuration for the `TaskLoop`.
    - [`taskRef` or `taskSpec`](#specifying-the-target-task) - Specifies the Task to execute.
    - [`iterateParam`](#specifying-the-iteration-parameter) - Specifies the name of the Task parameter that holds the values to iterate.
- Optional:
  - [`timeout`](#specifying-a-timeout) - Specifies a timeout for the execution of a Task.
  - [`retries`](#specifying-retries) - Specifies the number of times to retry the execution of a Task after a failure.

The example below shows a basic `TaskLoop`:

```yaml
apiVersion: custom.tekton.dev/v1alpha1
kind: TaskLoop
metadata:
  name: echoloop
spec:
  taskSpec:
    params:
      - name: message
        type: string
    steps:
      - name: echo
        image: ubuntu
        script: |
          #!/usr/bin/env bash
          echo "$(params.message)"
  iterateParam: message
```

#### Specifying the target task

To specify the `Task` you want to execute, use the `taskRef` field as shown below:

```yaml
spec:
  taskRef:
    name: message-task
```

You can also embed the `Task` definition directly using the `taskSpec` field:

```yaml
spec:
  taskSpec:
    params:
      - name: message
        type: string
    steps:
      - name: echo
        image: ubuntu
        script: |
          #!/usr/bin/env bash
          echo "$(params.message)"
```


#### Specifying the iteration parameter

The `iterateParam` field specifies the name of the `Task` parameter which varies for each execution of the `Task`.
This is what controls the loop.

* The parameter type as defined in the `Task` must be `string`.
* The parameter value as defined in the `Run` must be an array.

For example, suppose you have a `Task` that runs tests based on a parameter name called `test-type`.

```yaml
spec:
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: testtask
spec:
  params:
    - name: test-type
      type: string
  steps:
    - name: run-test
      image: docker.hub/...
      args: ["$(params.test-type)"]
```

If you want to run this task for multiple test types, your `TaskLoop` would look like this:

```yaml
apiVersion: custom.tekton.dev/v1alpha1
kind: TaskLoop
metadata:
  name: testloop
spec:
  taskRef:
    name: testtask
  iterateParam: test-type
```

Your `Run` would look like this:

```yaml
apiVersion: tekton.dev/v1alpha1
kind: Run
metadata:
  generateName: testloop-run-
spec:
  params:
    - name: test-type
      value:
        - codeanalysis
        - unittests
        - e2etests
  ref:
    apiVersion: custom.tekton.dev/v1alpha1
    kind: TaskLoop
    name: testloop
```

This `Run` would result in three `TaskRun`s being created to run the `Task` `testtask`.
In the first `TaskRun` the parameter `test-type` would be set to `codeanalysis`.
In the second `TaskRun` the parameter `test-type` would be set to `unittests`.
In the third `TaskRun` the parameter `test-type` would be set to `e2etests`.

#### Specifying a timeout

You can use the `timeout` field to set the TaskRun's timeout value.
If you do not specify this value for the TaskRun, the global default timeout value applies.
If you set the timeout to 0, the TaskRun will have no timeout and will run until it completes successfully or fails from an error.

The timeout value is a duration conforming to Go's ParseDuration format. For example, valid values are 1h30m, 1h, 1m, 60s, and 0.

See [Configuring the failure timeout](https://github.com/tektoncd/pipeline/blob/master/docs/taskruns.md#configuring-the-failure-timeout)
for more information about how TaskRun processes the timeout.

#### Specifying retries

You can specify the number of times to retry the execution of a `Task` when it fails.
If you don't explicitly specify a value, no retry is performed.

### Configuring a `Run`

A `Run` definition supports the following fields:

- Required:
  - [`apiVersion`][kubernetes-overview] - Specifies the API version, `tekton.dev/v1alpha1`.
  - [`kind`][kubernetes-overview] - Identifies this resource object as a `Run` object.
  - [`metadata`][kubernetes-overview] - Specifies the metadata that uniquely identifies the `Run`, such as a `name`.
  - [`spec`][kubernetes-overview] - Specifies the configuration for the `Run`.
    - [`ref`](#specifying-the-taskloop) - Specifies the type and name of the `TaskLoop` to execute.
    - [`params`](#specifying-parameters) - Specifies the execution parameters for the `Task`.

[kubernetes-overview]:
  https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/#required-fields

### Specifying the TaskLoop

Your `Run` must reference a TaskLoop.  It must include the `apiVersion`, `kind` and `name` fields as shown below.

```yaml
apiVersion: tekton.dev/v1alpha1
kind: Run
metadata:
  generateName: run-
spec:
  params:
    ...
  ref:
    apiVersion: custom.tekton.dev/v1alpha1
    kind: TaskLoop
    name: mytaskloop
```

### Specifying parameters

Your `Run` can provide any parameters that are defined by the `Task` that is referenced by the `TaskLoop`.
The parameters are passed through as is to each `TaskRun` with the exception of the iteration parameter named by `iterateParam` in the `TaskLoop`.

* In the `Run`, the iteration parameter value must be an array.
* A `TaskRun` is created for each array element with the iterate parameter value set to the element.
* In the `Task` the iteration parameter type must be `string`.

## Limitations

## Uninstall

## Want to get involved?

Visit the [Tekton Community](https://github.com/tektoncd/community) project for an overview of our processes.
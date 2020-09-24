# Task Loop Extension

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/kubernetes/experimental/blob/master/LICENSE)

The Task Loop Extension for Tekton allows users to run a Task in a loop with varying parameter values.
This functionality is provided by a controller that implements the [Tekton Custom Task interface](https://github.com/tektoncd/pipeline/blob/master/docs/runs.md).

This is an **_experimental feature_**.  The purpose is to explore potential use cases for looping in Tekton and how they may be achieved.

## Install

## Usage

Two resources are required to run a Task in a loop:

* A `TaskLoop` defines the Task to run and how to iterate it.
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

#### Specifying the iteration parameter

#### Specifying a timeout

#### Specifying retries

### Configuring a `Run`

A `Run` definition supports the following fields:

- Required:
  - [`apiVersion`][kubernetes-overview] - Specifies the API version, `tekton.dev/v1alpha1`.
  - [`kind`][kubernetes-overview] - Identifies this resource object as a `Run` object.
  - [`metadata`][kubernetes-overview] - Specifies the metadata that uniquely identifies the `Run`, such as a `name`.
  - [`spec`][kubernetes-overview] - Specifies the configuration for the `Run`.
    - [`ref`](#specifying-the-target-custom-task) - Specifies the type and name of the `TaskLoop` to execute.
    - [`params`](#specifying-parameters) - Specifies the desired execution parameters for the custom task.

[kubernetes-overview]:
  https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/#required-fields

## Limitations

## Uninstall

## Want to get involved?

Visit the [Tekton Community](https://github.com/tektoncd/community) project for an overview of our processes.
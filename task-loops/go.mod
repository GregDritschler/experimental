module github.com/tektoncd/experimental/task-loops

go 1.13

require (
	github.com/ghodss/yaml v1.0.0
	github.com/google/go-cmp v0.5.2
	github.com/hashicorp/go-multierror v1.1.0
	github.com/tektoncd/pipeline v0.15.2
	go.opencensus.io v0.22.4
	go.uber.org/zap v1.15.0
	gomodules.xyz/jsonpatch/v2 v2.1.0
	k8s.io/api v0.18.7-rc.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	knative.dev/pkg v0.0.0-20200831162708-14fb2347fb77
)

// Pin k8s deps to 1.17.6
replace (
	github.com/tektoncd/pipeline => ../../pipeline
	k8s.io/api => k8s.io/api v0.17.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.6
	k8s.io/apiserver => k8s.io/apiserver v0.17.6
	k8s.io/client-go => k8s.io/client-go v0.17.6
	k8s.io/code-generator => k8s.io/code-generator v0.17.6
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200410145947-bcb3869e6f29
)

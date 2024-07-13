# function-auto-ready-all
[![CI](https://github.com/crossplane-contrib/function-auto-ready/actions/workflows/ci.yml/badge.svg)](https://github.com/crossplane-contrib/function-auto-ready/actions/workflows/ci.yml) ![GitHub release (latest SemVer)](https://img.shields.io/github/release/crossplane-contrib/function-auto-ready)

This is a form of original function-auto-ready that checks not only `Ready` condition of the resources inside the composition, but all conditions.

This [composition function][docs-functions] automatically detects composed
resources that are ready. It considers a composed resource ready if:

* Another function added the composed resource to the desired state.
* The composed resource appears in the observed state (i.e. it exists).
* The composed resource has the status condition `type: Ready`, `status: True`.

Crossplane considers a composite resource (XR) to be ready when all of its
desired composed resources are ready.

In this example, the [Go Templating][fn-go-templating] function is used to add
a desired composed resource - an Amazon Web Services S3 Bucket. Once Crossplane
has created the Bucket, the Auto Ready function will let Crossplane know when it
is ready. Because this XR only has one composed resource, the XR will become
ready when the Bucket becomes ready.

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: example
spec:
  compositeTypeRef:
    apiVersion: example.crossplane.io/v1beta1
    kind: XR
  mode: Pipeline
  pipeline:
  - step: create-a-bucket
    functionRef:
      name: function-go-templating
    input:
      apiVersion: gotemplating.fn.crossplane.io/v1beta1
      kind: GoTemplate
      source: Inline
      inline:
        template: |
          apiVersion: s3.aws.upbound.io/v1beta1
          kind: Bucket
          metadata:
            annotations:
              gotemplating.fn.crossplane.io/composition-resource-name: bucket
          spec:
            forProvider:
              region: {{ .observed.composite.resource.spec.region }}
  - step: automatically-detect-ready-composed-resources
    functionRef:
      name: function-auto-ready
```

See the [example](example) directory for an example you can run locally using
the Crossplane CLI:

```shell
$ crossplane beta render xr.yaml composition.yaml functions.yaml
```

See the [composition functions documentation][docs-functions] to learn more
about `crossplane beta render`.

## Developing this function

This function uses [Go][go], [Docker][docker], and the [Crossplane CLI][cli] to
build functions.

```shell
# Run code generation - see input/generate.go
$ go generate ./...

# Run tests - see fn_test.go
$ go test ./...

# Build the function's runtime image - see Dockerfile
$ docker build . --tag=runtime

# Build a function package - see package/crossplane.yaml
$ crossplane xpkg build -f package --embed-runtime-image=runtime
```

[docs-functions]: https://docs.crossplane.io/v1.14/concepts/composition-functions/
[fn-go-templating]: https://github.com/crossplane-contrib/function-go-templating/tree/main
[go]: https://go.dev
[docker]: https://www.docker.com
[cli]: https://docs.crossplane.io/latest/cli
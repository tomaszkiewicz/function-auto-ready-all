package main

import (
	"context"
	"github.com/crossplane/function-sdk-go/resource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
)

// Function returns whatever response you ask it to.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

// RunFunction runs the Function.
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error) {
	f.log.Info("Running Function", "tag", req.GetMeta().GetTag())

	rsp := response.To(req, response.DefaultTTL)

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composite resource from %T", req))
		return rsp, nil
	}
	log := f.log.WithValues(
		"xr-apiversion", oxr.Resource.GetAPIVersion(),
		"xr-kind", oxr.Resource.GetKind(),
		"xr-name", oxr.Resource.GetName(),
	)

	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composed resources from %T", req))
		return rsp, nil
	}

	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composed resources from %T", req))
		return rsp, nil
	}

	f.log.Debug("Found desired resources", "count", len(desired))

	// Our goal here is to automatically determine (from the Ready status
	// condition) whether existing composed resources are ready.

	for name, dr := range desired {
		log := log.WithValues("composed-resource-name", name)

		or, ok := observed[name]
		if !ok {
			log.Debug("Ignoring desired resource that does not appear in observed resources")
			continue
		}

		if dr.Ready != resource.ReadyUnspecified {
			log.Debug("Ignoring desired resource that already has explicit readiness", "ready", dr.Ready)
			continue
		}

		log.Debug("Found desired resource with unknown readiness")

		// Extract conditions from the unstructured resource
		conditions, found, err := unstructured.NestedSlice(or.Resource.Object, "status", "conditions")
		if err != nil {
			log.Debug("Error getting conditions", "error", err)
			continue
		}
		if !found {
			log.Debug("No conditions found in resource")
			continue
		}

		allTrue := true
		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				log.Debug("Invalid condition format")
				allTrue = false
				break
			}

			status, ok := condition["status"].(string)
			if !ok {
				log.Debug("Invalid status format in condition")
				allTrue = false
				break
			}

			if status != string(corev1.ConditionTrue) {
				allTrue = false
				log.Debug("Found condition that is not True", "condition", condition["type"], "status", status)
				break
			}
		}

		if allTrue && len(conditions) > 0 {
			log.Info("Automatically determined that composed resource is ready (all conditions are True)")
			dr.Ready = resource.ReadyTrue
		} else {
			log.Info("Resource is not ready (not all conditions are True)")
		}
	}

	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources from %T", req))
		return rsp, nil
	}

	return rsp, nil
}

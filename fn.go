package main

import (
	"context"
	"fmt"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
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

	xr, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composite resource from %T", req))
		return rsp, nil
	}

	f.log.Debug("Found desired resources", "count", len(desired))

	// Our goal here is to automatically determine (from the Ready status
	// condition) whether existing composed resources are ready.

	erroredFields := []string{}

	for name, _ := range desired {
		log := log.WithValues("composed-resource-name", name)

		or, ok := observed[name]
		if !ok {
			log.Debug("Ignoring desired resource that does not appear in observed resources")
			continue
		}

		//if dr.Ready != resource.ReadyUnspecified {
		//	log.Debug("Ignoring desired resource that already has explicit readiness", "ready", dr.Ready)
		//	continue
		//}

		//log.Debug("Found desired resource with unknown readiness")

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

		//allTrue := true
		drErroredFields := []string{} // we handle errors per dr, so we can clean them when the resource is in Creating state
		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				log.Debug("Invalid condition format")
				//allTrue = false
				break
			}

			status, ok := condition["status"].(string)
			if !ok {
				log.Debug("Invalid status format in condition")
				//allTrue = false
				break
			}

			message, ok := condition["message"].(string)
			if !ok {
				message = ""
			}

			conditionType, ok := condition["type"].(string)
			if !ok {
				conditionType = ""
			}

			reason, ok := condition["reason"].(string)
			if !ok {
				reason = ""
			}

			if reason == "Creating" && conditionType == "Ready" {
				log.Info("Resource is in creating state, skipping Ready condition check")
				drErroredFields = []string{}
				break
			}

			if status != string(corev1.ConditionTrue) {
				//allTrue = false
				log.Info("Found condition that is not True", "condition", condition["type"], "status", status)
				drErroredFields = append(drErroredFields, fmt.Sprintf("\n=> %s %s=%s %s\n\n%s", name, condition["type"], status, reason, message))
				break
			}
		}

		erroredFields = append(erroredFields, drErroredFields...)

		//if allTrue && len(conditions) > 0 {
		//	log.Info("Automatically determined that composed resource is ready (all conditions are True)", "name", name)
		//	dr.Ready = resource.ReadyTrue
		//} else {
		//	log.Info("Resource is not ready (not all conditions are True)", "name", name)
		//}
	}

	//if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
	//	response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources from %T", req))
	//	return rsp, nil
	//}

	log.Info("Setting condition for XR composite condition")

	xrCondition := v1.Condition{
		Type:               "NoErrors",
		Status:             corev1.ConditionTrue,
		LastTransitionTime: v12.Now(),
		Reason:             v1.ReasonAvailable,
		Message:            "",
	}

	if len(erroredFields) > 0 {
		log.Info("Setting error for XR composite condition")

		xrCondition.Status = corev1.ConditionFalse
		xrCondition.Reason = v1.ReasonReconcileError
		xrCondition.Message = fmt.Sprintf("Unready conditions:\n %s", strings.Join(erroredFields, "\n"))
	}

	xr.Resource.SetConditions(xrCondition)

	if err := response.SetDesiredCompositeResource(rsp, xr); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composite resources from %T", req))
		return rsp, nil
	}

	return rsp, nil
}

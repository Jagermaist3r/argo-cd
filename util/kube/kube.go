package kube

import (
	"fmt"
	"regexp"

	"github.com/argoproj/gitops-engine/pkg/utils/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/argoproj/argo-cd/v3/common"
)

var resourceNamePattern = regexp.MustCompile("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")

// IsValidResourceName returns true if given string a valid Kubernetes resource name
func IsValidResourceName(name string) bool {
	return len(name) < 64 && resourceNamePattern.MatchString(name)
}

// SetAppInstanceLabel the recommended app.kubernetes.io/instance label against an unstructured object
// Uses the legacy labeling if environment variable is set
func SetAppInstanceLabel(target *unstructured.Unstructured, key, val string) error {
	labels, err := nestedNullableStringMap(target.Object, "metadata", "labels")
	if err != nil {
		return fmt.Errorf("failed to get labels from target object %s %s/%s: %w", target.GroupVersionKind().String(), target.GetNamespace(), target.GetName(), err)
	}
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = val
	target.SetLabels(labels)
	if key != common.LabelKeyLegacyApplicationName {
		// we no longer label the pod template sub resources in v0.11
		return nil
	}

	gvk := schema.FromAPIVersionAndKind(target.GetAPIVersion(), target.GetKind())
	// special case for deployment and job types: make sure that derived replicaset, and pod has
	// the application label
	switch gvk.Group {
	case "apps", "extensions":
		switch gvk.Kind {
		case kube.DeploymentKind, kube.ReplicaSetKind, kube.StatefulSetKind, kube.DaemonSetKind:
			templateLabels, ok, err := unstructured.NestedMap(target.UnstructuredContent(), "spec", "template", "metadata", "labels")
			if err != nil {
				return err
			}
			if !ok || templateLabels == nil {
				templateLabels = make(map[string]any)
			}
			templateLabels[key] = val
			err = unstructured.SetNestedMap(target.UnstructuredContent(), templateLabels, "spec", "template", "metadata", "labels")
			if err != nil {
				return err
			}
			// The following is a workaround for issue #335. In API version extensions/v1beta1 or
			// apps/v1beta1, if a spec omits spec.selector then k8s will default the
			// spec.selector.matchLabels to match spec.template.metadata.labels. This means Argo CD
			// labels can potentially make their way into spec.selector.matchLabels, which is a bad
			// thing. The following logic prevents this behavior.
			switch target.GetAPIVersion() {
			case "apps/v1beta1", "extensions/v1beta1":
				selector, _, err := unstructured.NestedMap(target.UnstructuredContent(), "spec", "selector")
				if err != nil {
					return err
				}
				if len(selector) == 0 {
					// If we get here, user did not set spec.selector in their manifest. We do not want
					// our Argo CD labels to get defaulted by kubernetes, so we explicitly set the labels
					// for them (minus the Argo CD labels).
					delete(templateLabels, key)
					err = unstructured.SetNestedMap(target.UnstructuredContent(), templateLabels, "spec", "selector", "matchLabels")
					if err != nil {
						return err
					}
				}
			}
		}
	case "batch":
		if gvk.Kind == kube.JobKind {
			templateLabels, ok, err := unstructured.NestedMap(target.UnstructuredContent(), "spec", "template", "metadata", "labels")
			if err != nil {
				return err
			}
			if !ok || templateLabels == nil {
				templateLabels = make(map[string]any)
			}
			templateLabels[key] = val
			err = unstructured.SetNestedMap(target.UnstructuredContent(), templateLabels, "spec", "template", "metadata", "labels")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// SetAppInstanceAnnotation the recommended app.kubernetes.io/instance annotation against an unstructured object
// Uses the legacy labeling if environment variable is set
func SetAppInstanceAnnotation(target *unstructured.Unstructured, key, val string) error {
	annotations, err := nestedNullableStringMap(target.Object, "metadata", "annotations")
	if err != nil {
		return fmt.Errorf("failed to get annotations from target object %s %s/%s: %w", target.GroupVersionKind().String(), target.GetNamespace(), target.GetName(), err)
	}

	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = val
	target.SetAnnotations(annotations)
	return nil
}

// GetAppInstanceAnnotation returns the application instance name from annotation
func GetAppInstanceAnnotation(un *unstructured.Unstructured, key string) (string, error) {
	annotations, err := nestedNullableStringMap(un.Object, "metadata", "annotations")
	if err != nil {
		return "", fmt.Errorf("failed to get annotations from target object %s %s/%s: %w", un.GroupVersionKind().String(), un.GetNamespace(), un.GetName(), err)
	}
	if annotations != nil {
		return annotations[key], nil
	}
	return "", nil
}

// GetAppInstanceLabel returns the application instance name from labels
func GetAppInstanceLabel(un *unstructured.Unstructured, key string) (string, error) {
	labels, err := nestedNullableStringMap(un.Object, "metadata", "labels")
	if err != nil {
		return "", fmt.Errorf("failed to get labels for %s %s/%s: %w", un.GroupVersionKind().String(), un.GetNamespace(), un.GetName(), err)
	}
	if labels != nil {
		return labels[key], nil
	}
	return "", nil
}

// RemoveLabel removes label with the specified name
func RemoveLabel(un *unstructured.Unstructured, key string) error {
	labels, err := nestedNullableStringMap(un.Object, "metadata", "labels")
	if err != nil {
		return fmt.Errorf("failed to get labels for %s %s/%s: %w", un.GroupVersionKind().String(), un.GetNamespace(), un.GetName(), err)
	}
	if labels == nil {
		return nil
	}

	for k := range labels {
		if k == key {
			delete(labels, k)
			if len(labels) == 0 {
				un.SetLabels(nil)
			} else {
				un.SetLabels(labels)
			}
			break
		}
	}
	return nil
}

// RemoveAnnotation removes annotation with the specified name
func RemoveAnnotation(un *unstructured.Unstructured, key string) error {
	annotations, err := nestedNullableStringMap(un.Object, "metadata", "annotations")
	if err != nil {
		return fmt.Errorf("failed to get annotations for %s %s/%s: %w", un.GroupVersionKind().String(), un.GetNamespace(), un.GetName(), err)
	}
	if annotations == nil {
		return nil
	}

	for k := range annotations {
		if k == key {
			delete(annotations, k)
			if len(annotations) == 0 {
				un.SetAnnotations(nil)
			} else {
				un.SetAnnotations(annotations)
			}
			break
		}
	}
	return nil
}

// nestedNullableStringMap returns a copy of map[string]string value of a nested field.
// Returns false if value is not found and an error if not one of map[string]any or nil, or contains non-string values in the map.
func nestedNullableStringMap(obj map[string]any, fields ...string) (map[string]string, error) {
	var m map[string]string
	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil {
		return nil, err
	}
	if found && val != nil {
		m, _, err = unstructured.NestedStringMap(obj, fields...)
	}
	return m, err
}

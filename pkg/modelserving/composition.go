package modelserving

import (
	"context"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// An Inference Service is its workload and the companions created next to
// it, each named <Inference Service name><suffix> and labelled with that name.
const (
	routerSuffix        = "-router"
	ingressSuffix       = "-gateway"
	storageSecretSuffix = "-model-storage"
	datasetSuffix       = "-model"
)

type companion struct {
	gvk    schema.GroupVersionKind
	suffix string
}

var ingressCompanion = companion{networkingv1.SchemeGroupVersion.WithKind("Ingress"), ingressSuffix}

// serviceCompanions are every companion an Inference Service may have, in
// the order they are deleted, the reverse of creation:
//   - the router with its RBAC objects, only under PD disaggregation;
//   - the Gateway route, only once Published;
//   - the OpenAI Service, always;
//   - the object storage Secret, only for s3 models;
//   - the reference to a Fluid Dataset, only for dataset models in another
//     namespace;
//   - the PodMonitor, only when the cluster has its CRD.
var serviceCompanions = []companion{
	{appsv1.SchemeGroupVersion.WithKind("Deployment"), routerSuffix},
	{rbacv1.SchemeGroupVersion.WithKind("RoleBinding"), routerSuffix},
	{rbacv1.SchemeGroupVersion.WithKind("Role"), routerSuffix},
	{corev1.SchemeGroupVersion.WithKind("ServiceAccount"), routerSuffix},
	ingressCompanion,
	{corev1.SchemeGroupVersion.WithKind("Service"), ""},
	{corev1.SchemeGroupVersion.WithKind("Secret"), storageSecretSuffix},
	{datasetGVK, datasetSuffix},
	{podMonitorGVK, ""},
}

// isDisaggregated reports whether an Inference Service's workload runs it
// under PD disaggregation. A request says so in its Deployment Mode instead.
func isDisaggregated(workload *unstructured.Unstructured) bool {
	return workload.GetKind() == disaggregatedSetGVK.Kind
}

// buildServiceObjects returns everything a service consists of in creation
// order, the workload last so its pods find their Secret and Service.
func buildServiceObjects(req *serviceRequest, served *model.ServedModel, storage *model.ModelStorage) ([]client.Object, *unstructured.Unstructured, error) {
	objects := []client.Object{}
	if storage != nil {
		objects = append(objects, buildStorageSecret(req, storage))
	}
	if reference := buildDatasetReference(req, served); reference != nil {
		objects = append(objects, reference)
	}
	objects = append(objects, buildService(req))
	build := buildLeaderWorkerSet
	if req.Mode == modePD {
		objects = append(objects, routerObjects(req)...)
		build = buildDisaggregatedSet
	}
	workload, err := build(req, served, storage)
	if err != nil {
		return nil, nil, err
	}
	return append(objects, workload), workload, nil
}

// buildDatasetReference returns the Dataset through which a service reads a
// dataset model kept in another namespace, as pods only mount the claims of
// their own; nil when it needs none. Fluid creates its claim, of the same name.
func buildDatasetReference(req *serviceRequest, served *model.ServedModel) *unstructured.Unstructured {
	namespace, _, _ := splitDataset(served.Dataset)
	if served.SourceType != sourceTypeDataset || namespace == req.Namespace {
		return nil
	}
	reference := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"mounts": []any{map[string]any{"mountPoint": datasetMountPrefix + served.Dataset}}},
	}}
	reference.SetGroupVersionKind(datasetGVK)
	reference.SetNamespace(req.Namespace)
	reference.SetName(datasetClaim(req, served))
	reference.SetLabels(serviceLabels(req.Name))
	return reference
}

// deleteServiceObjects deletes the workload with its companions. The workload
// goes last so a delete that failed halfway can be retried.
func deleteServiceObjects(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured) error {
	for _, companion := range serviceCompanions {
		if err := deleteCompanion(ctx, cs, workload, companion); err != nil {
			return err
		}
	}
	if err := cs.K8sClient.Delete(ctx, workload); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// deleteCompanion deletes a companion of the Inference Service whose workload
// is given. A same-named object without its label is not ours and stays.
func deleteCompanion(ctx context.Context, cs *cluster.ClientSet, workload *unstructured.Unstructured, companion companion) error {
	name := workload.GetName()
	// Unstructured reads skip the informer cache, which may not have seen a
	// companion that was only just created.
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(companion.gvk)
	err := cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: workload.GetNamespace(), Name: name + companion.suffix}, object)
	// Only the PodMonitor and Dataset kinds may be missing, in clusters
	// without their CRDs.
	optional := companion.gvk == podMonitorGVK || companion.gvk == datasetGVK
	if apierrors.IsNotFound(err) || (optional && apimeta.IsNoMatchError(err)) {
		return nil
	} else if err != nil {
		return err
	}
	if object.GetLabels()[serviceLabel] != name {
		return nil
	}
	return client.IgnoreNotFound(cs.K8sClient.Delete(ctx, object))
}

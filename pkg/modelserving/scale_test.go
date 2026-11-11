package modelserving

import (
	"context"
	"testing"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/kube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func builtWorkloads(t *testing.T) (lws, pd *unstructured.Unstructured) {
	t.Helper()
	single := parseTestRequest(t, singleBody())
	lws, err := buildLeaderWorkerSet(&single, chatModel(), nil)
	require.NoError(t, err)
	disaggregated := parseTestRequest(t, pdBody())
	pd, err = buildDisaggregatedSet(&disaggregated, chatModel(), nil)
	require.NoError(t, err)
	return lws, pd
}

func TestScaleWorkload(t *testing.T) {
	lws, pd := builtWorkloads(t)
	tests := []struct {
		name     string
		workload *unstructured.Unstructured
		req      scaleRequest
		wantErr  string
		want     map[string]int64
	}{
		{name: "leader worker set", workload: lws, req: scaleRequest{Replicas: 3}, want: map[string]int64{roleDefault: 3}},
		{name: "leader worker set without replicas", workload: lws, req: scaleRequest{}, wantErr: "replicas must be at least 1"},
		{
			name: "pd roles", workload: pd,
			req:  scaleRequest{PrefillReplicas: 2, DecodeReplicas: 4, RouterReplicas: new(int32(2))},
			want: map[string]int64{rolePrefill: 2, roleDecode: 4},
		},
		{
			name: "pd leaving the router alone", workload: pd,
			req:  scaleRequest{PrefillReplicas: 2, DecodeReplicas: 4},
			want: map[string]int64{rolePrefill: 2, roleDecode: 4},
		},
		{
			name: "pd without router replicas", workload: pd,
			req:     scaleRequest{PrefillReplicas: 2, DecodeReplicas: 4, RouterReplicas: new(int32(0))},
			wantErr: "routerReplicas must be at least 1",
		},
		{
			name: "pd without decode replicas", workload: pd,
			req:     scaleRequest{PrefillReplicas: 2, RouterReplicas: new(int32(1))},
			wantErr: "prefillReplicas and decodeReplicas must be at least 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.workload.DeepCopy()

			scaled, err := scaleWorkload(tt.workload, &tt.req)

			assert.Equal(t, before, tt.workload, "the workload must not change")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			for role, want := range tt.want {
				var replicas int64
				if role == roleDefault {
					replicas, _, _ = unstructured.NestedInt64(scaled.Object, "spec", "replicas")
				} else {
					replicas, _, _ = unstructured.NestedInt64(pdRoleSpecs(scaled)[role], "replicas")
				}
				assert.Equal(t, want, replicas, role)
			}
		})
	}
}

func fakeClientSet(t *testing.T, objects ...client.Object) *cluster.ClientSet {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	return &cluster.ClientSet{Name: "test", K8sClient: &kube.K8sClient{Client: fakeClient}}
}

func TestGetRouter(t *testing.T) {
	req := parseTestRequest(t, pdBody())
	_, workload := builtWorkloads(t)
	foreign := routerDeployment(t, &req)
	foreign.Labels = map[string]string{serviceLabel: "someone-else"}
	tests := []struct {
		name    string
		objects []client.Object
		found   bool
	}{
		{name: "ours", objects: []client.Object{routerDeployment(t, &req)}, found: true},
		{name: "missing", found: false},
		{name: "same name without our label", objects: []client.Object{foreign}, found: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fakeClientSet(t, tt.objects...)

			router, err := getRouter(context.Background(), cs, workload)

			require.NoError(t, err)
			assert.Equal(t, tt.found, router != nil)
		})
	}
}

func TestScaleRouter(t *testing.T) {
	req := parseTestRequest(t, pdBody())
	_, workload := builtWorkloads(t)
	cs := fakeClientSet(t, routerDeployment(t, &req))
	ctx := context.Background()
	router, err := getRouter(ctx, cs, workload)
	require.NoError(t, err)
	require.NotNil(t, router)

	require.NoError(t, scaleRouter(ctx, cs, router, 3))

	var scaled appsv1.Deployment
	require.NoError(t, cs.K8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "qwen-pd-router"}, &scaled))
	assert.Equal(t, int32(3), *scaled.Spec.Replicas)
	assert.Equal(t, router.Spec.Template, scaled.Spec.Template)
	assert.Equal(t, int32(1), *router.Spec.Replicas, "the router read before must not change")
}

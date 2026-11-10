package logging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/common"
	"github.com/qzytwway/mplatform/pkg/kube"
	"github.com/qzytwway/mplatform/pkg/model"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgofake "k8s.io/client-go/kubernetes/fake"
)

func TestBuildLogsQL(t *testing.T) {
	testcases := []struct {
		keyword string
		pod     string
		want    string
	}{
		{"", "", "*"},
		{"timeout", "", `"timeout"`},
		{`foo" OR * | delete`, "", `"foo\" OR * | delete"`},
		{"", "web-", `kubernetes.pod_name:="web-"*`},
		{"error", `we"b`, `kubernetes.pod_name:="we\"b"* "error"`},
	}
	for _, tc := range testcases {
		require.Equal(t, tc.want, buildLogsQL(tc.keyword, tc.pod))
	}
}

func logReaderRole(namespaces ...string) []common.Role {
	return []common.Role{{
		Name:       "log-reader",
		Clusters:   []string{"prod"},
		Namespaces: namespaces,
		Resources:  []string{"pods"},
		Verbs:      []string{"log"},
	}}
}

func TestParseLogQueryScopesNamespacesByRBAC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clientSet := &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{
		ClientSet: clientgofake.NewSimpleClientset(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-b"}},
		),
	}}
	const window = "start=2026-11-10T07:00:00Z&end=2026-11-10T08:00:00Z"

	testcases := []struct {
		name        string
		roles       []common.Role
		query       string
		wantStatus  int
		wantQuery   bool
		wantFilters map[string]any
	}{
		{
			name:        "all namespaces adds only the cluster",
			roles:       logReaderRole("*"),
			query:       window + "&namespace=_all",
			wantStatus:  http.StatusOK,
			wantQuery:   true,
			wantFilters: map[string]any{"cluster": "prod"},
		},
		{
			name:        "limited user gets readable namespaces",
			roles:       logReaderRole("team-a"),
			query:       window + "&namespace=_all&container=web",
			wantStatus:  http.StatusOK,
			wantQuery:   true,
			wantFilters: map[string]any{"cluster": "prod", "kubernetes.pod_namespace": []any{"team-a"}, "kubernetes.container_name": "web"},
		},
		{
			name:       "excluded namespace falls back to the list",
			roles:      logReaderRole("*", "!team-b"),
			query:      window,
			wantStatus: http.StatusOK,
			wantQuery:  true,
			wantFilters: map[string]any{
				"cluster": "prod", "kubernetes.pod_namespace": []any{"team-a"},
			},
		},
		{
			name:       "selected namespace must be readable",
			roles:      logReaderRole("team-a"),
			query:      window + "&namespace=team-b",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "no readable namespace returns no query",
			roles:      []common.Role{},
			query:      window,
			wantStatus: http.StatusOK,
		},
		{
			name:       "end must follow start",
			roles:      logReaderRole("*"),
			query:      "start=2026-11-10T08:00:00Z&end=2026-11-10T07:00:00Z",
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/logging/query?"+tc.query, nil)
			c.Set("cluster", clientSet)
			c.Set("user", model.User{Username: "alice", Roles: tc.roles})

			q, ok := parseLogQuery(c)

			require.Equal(t, tc.wantStatus, recorder.Code)
			require.Equal(t, tc.wantStatus == http.StatusOK, ok)
			if !tc.wantQuery {
				require.Nil(t, q)
				return
			}
			var filters map[string]any
			require.NoError(t, json.Unmarshal([]byte(q.extraFilters), &filters))
			require.Equal(t, tc.wantFilters, filters)
		})
	}
}

func TestAuthSecretIsCreatedUpdatedAndDeleted(t *testing.T) {
	ctx := context.Background()
	clientSet := &cluster.ClientSet{Name: "prod", K8sClient: &kube.K8sClient{ClientSet: clientgofake.NewSimpleClientset()}}
	secrets := clientSet.K8sClient.ClientSet.CoreV1().Secrets(Namespace)

	require.NoError(t, ensureAuthSecret(ctx, clientSet, "first"))
	require.NoError(t, ensureAuthSecret(ctx, clientSet, "second"))
	secret, err := secrets.Get(ctx, authSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "second", secret.StringData[authSecretKey])

	require.NoError(t, deleteAuthSecret(ctx, clientSet))
	require.NoError(t, deleteAuthSecret(ctx, clientSet))
	_, err = secrets.Get(ctx, authSecretName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

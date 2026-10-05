package controller

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
	"github.com/eliotstocker/webapp-operator/internal/cache"
	"github.com/eliotstocker/webapp-operator/internal/oci"
	"github.com/eliotstocker/webapp-operator/internal/server"
	"github.com/eliotstocker/webapp-operator/internal/syncer"
)

func TestWebsiteRevisionReconcilerLeader(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = webappv1alpha1.AddToScheme(scheme)
	_ = discoveryv1.AddToScheme(scheme)

	website := &webappv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site-1",
			Namespace: "default",
		},
		Spec: webappv1alpha1.WebsiteSpec{
			Image:     "ghcr.io/org/site-1:v1.0.0",
			Hostnames: []string{"site-1.example.com"},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{}`)},
	}

	revision := &webappv1alpha1.WebsiteRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site-1-rev1",
			Namespace: "default",
		},
		Spec: webappv1alpha1.WebsiteRevisionSpec{
			WebsiteName: "site-1",
			Image:       "ghcr.io/org/site-1:v1.0.0",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "my-secret"},
			},
		},
		Status: webappv1alpha1.WebsiteRevisionStatus{
			Phase: webappv1alpha1.RevisionPhasePending,
		},
	}

	// Create EndpointSlice with one ready pod and one terminating pod
	readyTrue := true
	termTrue := true
	epSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webapp-operator-abc",
			Namespace: "default",
			Labels:    map[string]string{discoveryv1.LabelServiceName: "webapp-operator"},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				TargetRef:  &corev1.ObjectReference{Kind: "Pod", Name: "gateway-pod-1"},
				Conditions: discoveryv1.EndpointConditions{Ready: &readyTrue},
			},
			{
				TargetRef:  &corev1.ObjectReference{Kind: "Pod", Name: "gateway-pod-terminating"},
				Conditions: discoveryv1.EndpointConditions{Terminating: &termTrue},
			},
		},
	}

	fakeClient := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(website, revision, secret, epSlice).
		WithStatusSubresource(&webappv1alpha1.Website{}, &webappv1alpha1.WebsiteRevision{}).
		Build()

	cacheDir := t.TempDir()
	cacheMgr, _ := cache.NewDiskManager(cacheDir)
	revDir := cacheMgr.RevisionDir("default", "site-1", "site-1-rev1")
	_ = os.MkdirAll(revDir, 0755)
	_ = os.WriteFile(filepath.Join(revDir, "index.html"), []byte("<h1>Cached</h1>"), 0644)

	router := server.NewRouter(cacheMgr)
	tracker := syncer.NewTracker()

	reconciler := &WebsiteRevisionReconciler{
		Client:            fakeClient,
		Scheme:            scheme,
		CacheManager:      cacheMgr,
		Puller:            oci.NewPuller(),
		Tracker:           tracker,
		Router:            router,
		PodName:           "gateway-pod-1",
		OperatorNamespace: "default",
		ServiceName:       "webapp-operator",
		IsLeader:          func() bool { return true },
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "site-1-rev1",
		},
	}

	_, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// Verify revision status updated to Active
	var updatedRev webappv1alpha1.WebsiteRevision
	if err := fakeClient.Get(ctx, req.NamespacedName, &updatedRev); err != nil {
		t.Fatalf("failed to get revision: %v", err)
	}

	if updatedRev.Status.Phase != webappv1alpha1.RevisionPhaseActive {
		t.Errorf("expected phase Active, got %s", updatedRev.Status.Phase)
	}
}

func TestWebsiteRevisionReconcilerReplica(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = webappv1alpha1.AddToScheme(scheme)

	revision := &webappv1alpha1.WebsiteRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "site-2-rev1",
			Namespace: "default",
		},
		Spec: webappv1alpha1.WebsiteRevisionSpec{
			WebsiteName: "site-2",
			Image:       "ghcr.io/org/site-2:v1.0.0",
		},
	}

	fakeClient := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(revision).
		WithStatusSubresource(&webappv1alpha1.WebsiteRevision{}).
		Build()

	cacheDir := t.TempDir()
	cacheMgr, _ := cache.NewDiskManager(cacheDir)
	revDir := cacheMgr.RevisionDir("default", "site-2", "site-2-rev1")
	_ = os.MkdirAll(revDir, 0755)
	_ = os.WriteFile(filepath.Join(revDir, "index.html"), []byte("ok"), 0644)

	leaderTracker := syncer.NewTracker()
	leaderServer := httptest.NewServer(leaderTracker)
	defer leaderServer.Close()

	reconciler := &WebsiteRevisionReconciler{
		Client:            fakeClient,
		Scheme:            scheme,
		CacheManager:      cacheMgr,
		Puller:            oci.NewPuller(),
		SyncerClient:      syncer.NewClient(),
		PodName:           "replica-pod-2",
		OperatorNamespace: "default",
		IsLeader:          func() bool { return false },
		GetLeaderAddress: func(ctx context.Context) (string, error) {
			return leaderServer.Listener.Addr().String(), nil
		},
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "site-2-rev1",
		},
	}

	_, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("replica reconcile failed: %v", err)
	}

	// Verify leader received the ping from replica
	synced := leaderTracker.GetReadyPods("default", "site-2-rev1")
	if len(synced) != 1 || synced[0] != "replica-pod-2" {
		t.Errorf("expected replica-pod-2 recorded on leader, got %v", synced)
	}
}

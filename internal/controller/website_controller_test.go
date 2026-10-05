package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
	"github.com/eliotstocker/webapp-operator/internal/cache"
	"github.com/eliotstocker/webapp-operator/internal/server"
)

func TestWebsiteReconcilerCreatesRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = webappv1alpha1.AddToScheme(scheme)

	website := &webappv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-site",
			Namespace: "default",
		},
		Spec: webappv1alpha1.WebsiteSpec{
			Image:     "ghcr.io/org/test-site:v1.0.0",
			Hostnames: []string{"test.example.com"},
			Env: map[string]string{
				"VITE_API": "https://api.test",
			},
			Injection: webappv1alpha1.InjectionSpec{
				Mode: "endpoint",
				Path: "/_config.js",
			},
		},
	}

	fakeClient := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(website).
		WithStatusSubresource(&webappv1alpha1.Website{}, &webappv1alpha1.WebsiteRevision{}).
		Build()

	router := server.NewRouter(nil)
	reconciler := &WebsiteReconciler{
		Client:      fakeClient,
		Scheme:      scheme,
		Router:      router,
		ServiceName: "webapp-operator",
		ServicePort: 80,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-site",
		},
	}

	// First reconcile: adds finalizer and creates child WebsiteRevision
	_, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// List WebsiteRevisions
	var revList webappv1alpha1.WebsiteRevisionList
	if err := fakeClient.List(ctx, &revList); err != nil {
		t.Fatalf("failed to list revisions: %v", err)
	}

	if len(revList.Items) != 1 {
		t.Fatalf("expected 1 WebsiteRevision created, got %d", len(revList.Items))
	}

	rev := revList.Items[0]
	if rev.Spec.WebsiteName != "test-site" {
		t.Errorf("expected WebsiteName 'test-site', got %s", rev.Spec.WebsiteName)
	}
	if rev.Spec.Image != "ghcr.io/org/test-site:v1.0.0" {
		t.Errorf("expected Image 'ghcr.io/org/test-site:v1.0.0', got %s", rev.Spec.Image)
	}
	if rev.Spec.Env["VITE_API"] != "https://api.test" {
		t.Errorf("expected env var VITE_API, got %v", rev.Spec.Env)
	}
}

func TestWebsiteReconcilerIngress(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = webappv1alpha1.AddToScheme(scheme)

	website := &webappv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress-site",
			Namespace: "default",
		},
		Spec: webappv1alpha1.WebsiteSpec{
			Image:     "ghcr.io/org/ingress-site:v1.0.0",
			Hostnames: []string{"site.example.com"},
			Ingress: &webappv1alpha1.IngressSpec{
				Enabled: true,
			},
		},
	}

	fakeClient := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(website).
		WithStatusSubresource(&webappv1alpha1.Website{}, &webappv1alpha1.WebsiteRevision{}).
		Build()

	reconciler := &WebsiteReconciler{
		Client:      fakeClient,
		Scheme:      scheme,
		ServiceName: "webapp-operator",
		ServicePort: 80,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "ingress-site",
		},
	}

	_, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
}

func TestWebsiteReconcilerPruningAndDeletion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = webappv1alpha1.AddToScheme(scheme)

	limit := int32(2)
	website := &webappv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "prune-site",
			Namespace:  "default",
			Finalizers: []string{websiteFinalizer},
		},
		Spec: webappv1alpha1.WebsiteSpec{
			Image:                "ghcr.io/org/prune:v3",
			Hostnames:            []string{"prune.example.com"},
			RevisionHistoryLimit: &limit,
		},
		Status: webappv1alpha1.WebsiteStatus{
			ActiveRevision: "rev-2",
		},
	}

	// Create 3 revisions
	rev1 := &webappv1alpha1.WebsiteRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rev-1",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
		},
		Spec: webappv1alpha1.WebsiteRevisionSpec{WebsiteName: "prune-site", Image: "ghcr.io/org/prune:v1"},
	}
	rev2 := &webappv1alpha1.WebsiteRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rev-2",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-20 * time.Minute)),
		},
		Spec: webappv1alpha1.WebsiteRevisionSpec{WebsiteName: "prune-site", Image: "ghcr.io/org/prune:v2"},
	}
	rev3 := &webappv1alpha1.WebsiteRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rev-3",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		},
		Spec: webappv1alpha1.WebsiteRevisionSpec{WebsiteName: "prune-site", Image: "ghcr.io/org/prune:v3"},
	}

	fakeClient := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(website, rev1, rev2, rev3).
		WithStatusSubresource(&webappv1alpha1.Website{}, &webappv1alpha1.WebsiteRevision{}).
		Build()

	cacheDir := t.TempDir()
	cacheMgr, _ := cache.NewDiskManager(cacheDir)
	router := server.NewRouter(cacheMgr)

	reconciler := &WebsiteReconciler{
		Client:       fakeClient,
		Scheme:       scheme,
		Router:       router,
		CacheManager: cacheMgr,
		ServiceName:  "webapp-operator",
		ServicePort:  80,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "prune-site",
		},
	}

	// Reconcile triggers pruning of rev-1 (keeps rev-2 because active, keeps rev-3 as newest)
	_, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// Verify rev-1 was deleted
	var remaining webappv1alpha1.WebsiteRevisionList
	_ = fakeClient.List(ctx, &remaining)
	for _, r := range remaining.Items {
		if r.Name == "rev-1" {
			t.Errorf("expected rev-1 to be pruned")
		}
	}

	// 2. Test Deletion with finalizer
	_ = fakeClient.Delete(ctx, website)

	_, err = reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("deletion reconcile failed: %v", err)
	}

	var updatedWebsite webappv1alpha1.Website
	err = fakeClient.Get(ctx, req.NamespacedName, &updatedWebsite)
	if err == nil && controllerutil.ContainsFinalizer(&updatedWebsite, websiteFinalizer) {
		t.Errorf("expected finalizer to be removed on deletion")
	}
}

func init() {
	// Suppress unused imports if any
	_ = fmt.Sprintf("")
}

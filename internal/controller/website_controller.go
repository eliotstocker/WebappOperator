package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
	"github.com/eliotstocker/webapp-operator/internal/cache"
	"github.com/eliotstocker/webapp-operator/internal/server"
)

const websiteFinalizer = "webapp.io/finalizer"

// WebsiteReconciler reconciles a Website object
type WebsiteReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Router       *server.Router
	CacheManager cache.Manager
	ServiceName  string
	ServicePort  int32
}

// +kubebuilder:rbac:groups=webapp.io,resources=websites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=webapp.io,resources=websites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=webapp.io,resources=websiterevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *WebsiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var website webappv1alpha1.Website
	if err := r.Get(ctx, req.NamespacedName, &website); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 1. Handle Deletion & Finalizer
	if !website.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&website, websiteFinalizer) {
			if r.Router != nil {
				r.Router.RemoveWebsiteRoutes(website.Spec.Hostnames)
			}
			controllerutil.RemoveFinalizer(&website, websiteFinalizer)
			return ctrl.Result{}, r.Update(ctx, &website)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&website, websiteFinalizer) {
		controllerutil.AddFinalizer(&website, websiteFinalizer)
		if err := r.Update(ctx, &website); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 2. Compute revision name based on spec hash
	revHash := computeSpecHash(website.Spec)
	revName := fmt.Sprintf("%s-%s", website.Name, revHash[:8])

	// 3. Ensure child WebsiteRevision exists
	var revision webappv1alpha1.WebsiteRevision
	err := r.Get(ctx, client.ObjectKey{Namespace: website.Namespace, Name: revName}, &revision)
	if errors.IsNotFound(err) {
		logger.Info("Creating new WebsiteRevision", "revision", revName)
		revision = webappv1alpha1.WebsiteRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      revName,
				Namespace: website.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(&website, webappv1alpha1.GroupVersion.WithKind("Website")),
				},
			},
			Spec: webappv1alpha1.WebsiteRevisionSpec{
				WebsiteName:      website.Name,
				Image:            website.Spec.Image,
				Env:              website.Spec.Env,
				Injection:        website.Spec.Injection,
				ImagePullSecrets: website.Spec.ImagePullSecrets,
			},
			Status: webappv1alpha1.WebsiteRevisionStatus{
				Phase: webappv1alpha1.RevisionPhasePending,
			},
		}
		if err := r.Create(ctx, &revision); err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// 4. Reconcile Ingress if enabled
	if website.Spec.Ingress != nil && website.Spec.Ingress.Enabled {
		if err := r.reconcileIngress(ctx, &website); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 5. Prune old WebsiteRevisions according to revisionHistoryLimit
	limit := int32(3)
	if website.Spec.RevisionHistoryLimit != nil {
		limit = *website.Spec.RevisionHistoryLimit
	}
	if err := r.pruneOldRevisions(ctx, &website, limit); err != nil {
		logger.Error(err, "Failed to prune old revisions", "website", website.Name)
	}

	// 6. Update router if active revision exists
	if website.Status.ActiveRevision != "" && r.Router != nil {
		target := server.RouteTarget{
			Namespace:           website.Namespace,
			WebsiteName:         website.Name,
			RevisionName:        website.Status.ActiveRevision,
			Image:               website.Status.ActiveImage,
			Env:                 website.Spec.Env,
			InjectionMode:       string(website.Spec.Injection.Mode),
			ConfigPath:          website.Spec.Injection.Path,
			VersionPath:         website.Spec.Injection.GetVersionPath(),
			DisablePolling:      !website.Spec.Injection.IsVersionPollingEnabled(),
			PollIntervalSeconds: website.Spec.Injection.GetPollIntervalSeconds(),
		}
		r.Router.SetWebsiteRoutes(website.Spec.Hostnames, target)
	}

	return ctrl.Result{}, nil
}

func (r *WebsiteReconciler) reconcileIngress(ctx context.Context, website *webappv1alpha1.Website) error {
	ingressName := fmt.Sprintf("%s-ingress", website.Name)
	pathType := networkingv1.PathTypePrefix
	servicePort := r.ServicePort
	if servicePort == 0 {
		servicePort = 80
	}
	serviceName := r.ServiceName
	if serviceName == "" {
		serviceName = "webapp-operator"
	}

	var rules []networkingv1.IngressRule
	for _, host := range website.Spec.Hostnames {
		rules = append(rules, networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: serviceName,
									Port: networkingv1.ServiceBackendPort{
										Number: servicePort,
									},
								},
							},
						},
					},
				},
			},
		})
	}

	desired := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressName,
			Namespace:   website.Namespace,
			Annotations: website.Spec.Ingress.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(website, webappv1alpha1.GroupVersion.WithKind("Website")),
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: website.Spec.Ingress.ClassName,
			TLS:              website.Spec.Ingress.TLS,
			Rules:            rules,
		},
	}

	var existing networkingv1.Ingress
	err := r.Get(ctx, client.ObjectKey{Namespace: website.Namespace, Name: ingressName}, &existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}

	existing.Spec = desired.Spec
	existing.Annotations = desired.Annotations
	return r.Update(ctx, &existing)
}

func (r *WebsiteReconciler) pruneOldRevisions(ctx context.Context, website *webappv1alpha1.Website, limit int32) error {
	var revList webappv1alpha1.WebsiteRevisionList
	if err := r.List(ctx, &revList, client.InNamespace(website.Namespace)); err != nil {
		return err
	}

	// Filter revisions belonging to this website
	var owned []webappv1alpha1.WebsiteRevision
	for _, rev := range revList.Items {
		if rev.Spec.WebsiteName == website.Name {
			owned = append(owned, rev)
		}
	}

	if int32(len(owned)) <= limit {
		return nil
	}

	// Sort oldest first
	sort.Slice(owned, func(i, j int) bool {
		return owned[i].CreationTimestamp.Before(&owned[j].CreationTimestamp)
	})

	excess := int32(len(owned)) - limit
	for i := 0; i < len(owned) && excess > 0; i++ {
		rev := owned[i]
		// Never prune the active revision
		if rev.Name == website.Status.ActiveRevision {
			continue
		}

		if err := r.Delete(ctx, &rev); err == nil {
			excess--
			if r.CacheManager != nil {
				_ = r.CacheManager.PruneRevision(rev.Namespace, rev.Spec.WebsiteName, rev.Name)
			}
		}
	}

	return nil
}

func (r *WebsiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1alpha1.Website{}).
		Owns(&webappv1alpha1.WebsiteRevision{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}

func computeSpecHash(spec webappv1alpha1.WebsiteSpec) string {
	data, _ := json.Marshal([]any{spec.Image, spec.Env, spec.Injection})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

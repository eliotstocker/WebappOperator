package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
	"github.com/eliotstocker/webapp-operator/internal/cache"
	"github.com/eliotstocker/webapp-operator/internal/oci"
	"github.com/eliotstocker/webapp-operator/internal/server"
	"github.com/eliotstocker/webapp-operator/internal/syncer"
)

// WebsiteRevisionReconciler reconciles a WebsiteRevision object
type WebsiteRevisionReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	CacheManager      cache.Manager
	Puller            oci.Puller
	SyncerClient      *syncer.Client
	Tracker           *syncer.Tracker
	Router            *server.Router
	PodName           string
	OperatorNamespace string
	ServiceName       string
	IsLeader          func() bool
	GetLeaderAddress  func(ctx context.Context) (string, error)
}

// +kubebuilder:rbac:groups=webapp.io,resources=websiterevisions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=webapp.io,resources=websiterevisions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *WebsiteRevisionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var revision webappv1alpha1.WebsiteRevision
	if err := r.Get(ctx, req.NamespacedName, &revision); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 1. Fetch referenced ImagePullSecrets if any
	var secrets []corev1.Secret
	for _, secRef := range revision.Spec.ImagePullSecrets {
		var sec corev1.Secret
		if err := r.Get(ctx, client.ObjectKey{Namespace: revision.Namespace, Name: secRef.Name}, &sec); err == nil {
			secrets = append(secrets, sec)
		}
	}

	// 2. Ensure revision assets are cached locally
	if !r.CacheManager.IsCached(revision.Namespace, revision.Spec.WebsiteName, revision.Name) {
		logger.Info("Pulling and caching revision locally", "revision", revision.Name, "image", revision.Spec.Image)
		_, err := r.CacheManager.EnsureRevision(ctx, revision.Namespace, revision.Spec.WebsiteName, revision.Name, func(destDir string) error {
			res, err := r.Puller.PullAndExtract(ctx, revision.Spec.Image, destDir, secrets)
			if err != nil {
				return err
			}
			if res != nil && revision.Spec.Digest == "" {
				revision.Spec.Digest = res.Digest
			}
			return nil
		})
		if err != nil {
			logger.Error(err, "Failed to pull and unpack revision", "revision", revision.Name)
			return ctrl.Result{}, err
		}
	}

	isLeader := r.IsLeader != nil && r.IsLeader()

	// 3. Register cache readiness
	if isLeader {
		r.Tracker.RecordReady(revision.Namespace, revision.Name, r.PodName)
	} else if r.SyncerClient != nil && r.GetLeaderAddress != nil {
		leaderAddr, err := r.GetLeaderAddress(ctx)
		if err == nil && leaderAddr != "" {
			_ = r.SyncerClient.NotifyLeader(ctx, leaderAddr, syncer.SyncRequest{
				PodName:   r.PodName,
				Namespace: revision.Namespace,
				Website:   revision.Spec.WebsiteName,
				Revision:  revision.Name,
			})
		}
	}

	// 4. Leader-only: Check EndpointSlice active pods vs ready pods
	if isLeader {
		return r.reconcileLeaderRollout(ctx, &revision)
	}

	return ctrl.Result{}, nil
}

func (r *WebsiteRevisionReconciler) reconcileLeaderRollout(ctx context.Context, revision *webappv1alpha1.WebsiteRevision) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch active gateway pods from EndpointSlices
	activePods := make(map[string]struct{})
	var epList discoveryv1.EndpointSliceList
	listOpts := []client.ListOption{
		client.InNamespace(r.OperatorNamespace),
	}
	if r.ServiceName != "" {
		listOpts = append(listOpts, client.MatchingLabels{discoveryv1.LabelServiceName: r.ServiceName})
	}

	if err := r.List(ctx, &epList, listOpts...); err == nil {
		for _, slice := range epList.Items {
			for _, ep := range slice.Endpoints {
				if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
					continue
				}
				if ep.Conditions.Terminating != nil && *ep.Conditions.Terminating {
					continue
				}
				if ep.TargetRef != nil && ep.TargetRef.Kind == "Pod" {
					activePods[ep.TargetRef.Name] = struct{}{}
				}
			}
		}
	}

	// Get pods that reported ready for this revision
	syncedPods := r.Tracker.GetReadyPods(revision.Namespace, revision.Name)
	syncedSet := make(map[string]struct{}, len(syncedPods))
	for _, p := range syncedPods {
		syncedSet[p] = struct{}{}
	}

	// Determine if Active Pods ⊆ Synced Pods
	allSynced := true
	if len(activePods) > 0 {
		for pod := range activePods {
			if _, ok := syncedSet[pod]; !ok {
				allSynced = false
				break
			}
		}
	} else {
		// Single replica / local dev fallback
		allSynced = len(syncedPods) > 0
	}

	revision.Status.ReadyPods = syncedPods
	if allSynced {
		revision.Status.Phase = webappv1alpha1.RevisionPhaseActive
	} else if len(syncedPods) > 0 {
		revision.Status.Phase = webappv1alpha1.RevisionPhasePrewarming
	}

	if err := r.Status().Update(ctx, revision); err != nil && !errors.IsConflict(err) {
		return ctrl.Result{}, err
	}

	// If revision is now active, update parent Website status and router
	if allSynced {
		var website webappv1alpha1.Website
		if err := r.Get(ctx, client.ObjectKey{Namespace: revision.Namespace, Name: revision.Spec.WebsiteName}, &website); err == nil {
			if website.Status.ActiveRevision != revision.Name {
				logger.Info("Promoting revision to Active", "website", website.Name, "revision", revision.Name)
				website.Status.ActiveRevision = revision.Name
				website.Status.ActiveImage = revision.Spec.Image
				website.Status.Phase = webappv1alpha1.WebsitePhaseReady
				_ = r.Status().Update(ctx, &website)
			}

			// Immediately update in-memory router routes
			if r.Router != nil {
				target := server.RouteTarget{
					Namespace:           website.Namespace,
					WebsiteName:         website.Name,
					RevisionName:        revision.Name,
					Image:               revision.Spec.Image,
					Env:                 revision.Spec.Env,
					InjectionMode:       string(revision.Spec.Injection.Mode),
					ConfigPath:          revision.Spec.Injection.Path,
					VersionPath:         revision.Spec.Injection.GetVersionPath(),
					DisablePolling:      !revision.Spec.Injection.IsVersionPollingEnabled(),
					PollIntervalSeconds: revision.Spec.Injection.GetPollIntervalSeconds(),
				}
				r.Router.SetWebsiteRoutes(website.Spec.Hostnames, target)
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *WebsiteRevisionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1alpha1.WebsiteRevision{}).
		Complete(r)
}

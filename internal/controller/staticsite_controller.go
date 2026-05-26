package controller

import (
	"context"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	demov1 "demo.io/api/v1"
)

// reconciler struct
type StaticSiteReconciler struct {
	client.Client // embedded the standard high performance k8s data client which allow our struct to access database commands
	Scheme        *runtime.Scheme
}

//+kubebuilder:rbac:groups=demo.io,resources=staticsites,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=demo.io,resources=staticsites/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile function - the main logic of the controller
// function trigger whenever the use create edit, delete the YAML file
func (r *StaticSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) { // ctrl.Request - tells the exact name and namespace of the object that changed
	l := log.FromContext(ctx) // create a logger to print logs in the terminal

	// 1. Fetch the StaticSite instance
	var staticSite demov1.StaticSite
	if err := r.Get(ctx, req.NamespacedName, &staticSite); err != nil {
		if errors.IsNotFound(err) { // if the object is not found (deleted) we just ignore it and return
			return ctrl.Result{}, nil
		}
		l.Error(err, "unable to fetch StaticSite") // if there is an error fetching the object we log it and requeue the request for later processing
		return ctrl.Result{}, err
	}

	// 2. creating and syncing the configmap
	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      staticSite.Name + "-html", // create a configmap with the name of the static site + "-html"
			Namespace: staticSite.Namespace,
		},
		Data: map[string]string{
			"index.html": staticSite.Spec.Content, // put the content of the static site in the configmap data
		},
	}

	// set Garbage Collection
	if err := ctrl.SetControllerReference(&staticSite, desiredConfigMap, r.Scheme); err != nil {
		return ctrl.Result{}, err // set the owner reference of the configmap to the static site, so that when the static site is deleted, the configmap will be deleted automatically
	}

	// 3. creating and syncing the configmap to the cluster
	// Idempotency logic. Verify if the child ConfigMap already exists in the cluster. if missing, create one. if present compares the data . If detected any changes, update the live config map instantly
	var existingConfigMap corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKey{Name: desiredConfigMap.Name, Namespace: desiredConfigMap.Namespace}, &existingConfigMap)

	if err != nil && errors.IsNotFound(err) {
		l.Info("Creating a new ConfigMap", "Namespace", desiredConfigMap.Namespace, "Name", desiredConfigMap.Name)
		if err := r.Create(ctx, desiredConfigMap); err != nil {
			l.Error(err, "Failed to create ConfigMap", "Namespace", desiredConfigMap.Namespace, "Name", desiredConfigMap.Name)
			return ctrl.Result{}, err
		}
	} else if err == nil { // if configMap already exists
		existingConfigMap.Data = desiredConfigMap.Data
		if err := r.Update(ctx, &existingConfigMap); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. define the desired Nginx Deployment
	desiredDeployment := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      staticSite.Name,
			Namespace: staticSite.Namespace,
		},
		Spec: appv1.DeploymentSpec{
			Replicas: &staticSite.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": staticSite.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": staticSite.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:latest",
						Ports: []corev1.ContainerPort{{ContainerPort: 80}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "html-volume",
							MountPath: "/usr/share/nginx/html",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "html-volume",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: desiredConfigMap.Name},
							},
						},
					}},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(&staticSite, desiredDeployment, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// 5. Apply/sync the Deployment
	// Idempotency logic in deployment creation and update
	var existingDeployment appv1.Deployment
	err = r.Get(ctx, client.ObjectKey{Name: desiredDeployment.Name, Namespace: desiredDeployment.Namespace}, &existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		l.Info("Creating a new Deployment", "Namespace", desiredDeployment.Namespace, "Name", desiredDeployment.Name)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return ctrl.Result{}, err
		}
	} else if err == nil {
		existingDeployment.Spec.Replicas = desiredDeployment.Spec.Replicas
		if err := r.Update(ctx, &existingDeployment); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 6. Define the desired Service
	desiredService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      staticSite.Name + "-service",
			Namespace: staticSite.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": staticSite.Name},
			Ports: []corev1.ServicePort{{
				Protocol:   corev1.ProtocolTCP,
				Port:       80,
				TargetPort: intstr.FromInt32(80),
			}},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(&staticSite, desiredService, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// 7. Apply/Sync the Service
	var existingService corev1.Service
	err = r.Get(ctx, client.ObjectKey{Name: desiredService.Name, Namespace: desiredService.Namespace}, &existingService)
	if err != nil && errors.IsNotFound(err) {
		l.Info("Creating a new Service")
		if err := r.Create(ctx, desiredService); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// registering the watcher
// setupWithManager tells the controller manager to watch our Custom Resource type
func (r *StaticSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1.StaticSite{}). // instruct the manager to run the reconcile function whenever a StaticSite object is added or modified
		Owns(&appv1.Deployment{}). // instruct the manager to run the reconcile function whenever a Deployment object owned by a StaticSite is added or modified (healing mechanism)
		Owns(&corev1.ConfigMap{}). // instruct the manager to run the reconcile function whenever a ConfigMap object owned by a StaticSite is added or modified (healing mechanism)
		Owns(&corev1.Service{}).   // instruct the manager to run the reconcile function whenever a Service object owned by a StaticSite is added or modified (healing mechanism)
		Complete(r)
}

/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	sidev1alpha1 "github.com/devnjw/sidecar/api/v1alpha1"
)

// SidecarReconciler reconciles a Sidecar object
type SidecarReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=side.sidecar.com,resources=sidecars,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=side.sidecar.com,resources=sidecars/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=side.sidecar.com,resources=sidecars/finalizers,verbs=update

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Sidecar object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *SidecarReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Fetch the Sidecar instance
	sidecar := &sidev1alpha1.Sidecar{}
	err := r.Get(ctx, req.NamespacedName, sidecar)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Memcached resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Memcached")
		return ctrl.Result{}, err
	}

	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: getServiceName(sidecar.Name), Namespace: sidecar.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		// Define a new service
		svc := r.serviceForSidecar(sidecar)
		log.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Deployment.Name", svc.Name)
		err = r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: sidecar.Name, Namespace: sidecar.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForSidecar(sidecar)
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Ask to requeue after 1 minute in order to give enough time for the
	// pods be created on the cluster side and the operand be able
	// to do the next update step accurately.
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// func (r *SidecarReconciler) pvForSidecar(s *sidev1alpha1.Sidecar) *corev1.PersistentVolume {
// 	ls := labelsForSidecar(s.Name)

// 	pv := &corev1.PersistentVolume{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      getServiceName(s.Name),
// 			Namespace: s.Namespace,
// 		},
// 		Spec: corev1.PersistentVolumeSpec{
// 			Capacity: corev1.ResourceList{
// 				"storage": resource.Quantity{"2Gi"},
// 			},
// 			VolumeMode: corev1.PersistentVolumeMode{"Filesystem"}
// 		},
// 	}
// }

func (r *SidecarReconciler) serviceForSidecar(s *sidev1alpha1.Sidecar) *corev1.Service {
	ls := labelsForSidecar(s.Name)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getServiceName(s.Name),
			Namespace: s.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: ls,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(80),
			}},
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}
	return svc
}

func (r *SidecarReconciler) deploymentForSidecar(s *sidev1alpha1.Sidecar) *appsv1.Deployment {
	ls := labelsForSidecar(s.Name)
	replicas := s.Spec.Size

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.Name,
			Namespace: s.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "sidecar",
						Image: "alicek106/rr-test:echo-hostname",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "sidecar-volume-hostpath",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/tmp",
							},
						},
					}},
				},
			},
		},
	}
	// Set Sidecar instance as the owner and controller
	ctrl.SetControllerReference(s, dep, r.Scheme)
	return dep
}

// labelsForMemcached returns the labels for selecting the resources
// belonging to the given memcached CR name.
func labelsForSidecar(name string) map[string]string {
	return map[string]string{"app": "sidecar-webserver"}
}

func getServiceName(name string) string {
	return "service-" + name
}

// SetupWithManager sets up the controller with the Manager.
func (r *SidecarReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sidev1alpha1.Sidecar{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

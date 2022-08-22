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
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	kmc "kmodules.xyz/client-go/client"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"

	apiserverv1alpha1 "github.com/Superm4n97/custom-controller/api/v1alpha1"
)

// BookServerReconciler reconciles a BookServer object
type BookServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func getCurrentTime() time.Time {
	return time.Now()
}

func getOwnerReference(bs *apiserverv1alpha1.BookServer) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: bs.APIVersion,
			Kind:       bs.Kind,
			Name:       bs.Name,
			UID:        bs.UID,
		},
	}
}

func constructNewDeploymentForBookServer(bs *apiserverv1alpha1.BookServer, depName string) (*appsv1.Deployment, error) {
	fmt.Println(depName)
	newDep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            depName,
			Labels:          bs.Labels,
			Namespace:       bs.Namespace,
			OwnerReferences: getOwnerReference(bs),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &bs.Spec.Selector,
			Replicas: bs.Spec.Replicas,

			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: bs.Spec.Selector.MatchLabels,
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:  "bookserver",
							Image: "superm4n/book-api-server:v0.1.3",
							Ports: []core.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	return &newDep, nil
}

//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
//+kubebuilder:rbac:groups=apiserver.example.com,resources=bookservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apiserver.example.com,resources=bookservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apiserver.example.com,resources=bookservers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BookServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *BookServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	key := req.NamespacedName

	klog.Info("Get event for BooksServer name: ", key.Name, " in namespace: ", key.Namespace)

	fmt.Println("Event occurred")

	bookServer := &apiserverv1alpha1.BookServer{}
	if err := r.Get(ctx, req.NamespacedName, bookServer); err != nil {
		fmt.Println("unable to fetch the book server")
		klog.Error(err, "unable to fetch the book server")

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	bookServer = bookServer.DeepCopy()

	// first time create / deployment missing --> new deployment create
	// deployment  --> check deployment selector, replicas match or not with BookServer

	fmt.Println("current book server replicas: ", *bookServer.Spec.Replicas)

	//create a deployment if not present
	//------------------------------------------------

	childName := bookServer.Status.ChildDeployment
	childDep := &appsv1.Deployment{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: bookServer.Name, Namespace: bookServer.Namespace}, childDep)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if childName == nil || client.IgnoreNotFound(err) == nil {
		st := fmt.Sprintf("%s-%d", bookServer.Name, getCurrentTime().Unix())
		childName = &st

		_, _, err = kmc.PatchStatus(ctx, r.Client, bookServer, func(obj client.Object) client.Object {
			in := obj.(*apiserverv1alpha1.BookServer)
			in.Status.Phase = apiserverv1alpha1.BookServerPending
			in.Status.ChildDeployment = childName
			return in
		})
		if err != nil {
			klog.Error("failed to patch the status first time")
			fmt.Println("failed to patch the status first time")
			return ctrl.Result{}, err
		}

		dep, err := constructNewDeploymentForBookServer(bookServer, *childName)
		if err != nil {
			klog.Error("failed to construct the deployment")
			fmt.Println("failed to construct the deployment")
			return ctrl.Result{}, err
		}

		_, _, err = kmc.CreateOrPatch(ctx, r.Client, dep, func(obj client.Object, createOp bool) client.Object {
			return dep
		})
		if err != nil {
			klog.Error("failed to create new deployment for first time")
			fmt.Println("failed to create new deployment for first time")
			return ctrl.Result{}, err
		}

	} else {
		_, _, err = kmc.CreateOrPatch(ctx, r.Client, childDep, func(obj client.Object, createOp bool) client.Object {
			in := obj.(*appsv1.Deployment)
			in.Spec.Replicas = bookServer.Spec.Replicas
			return in
		})
		if err != nil {
			klog.Error("failed to create or patch deployment")
			fmt.Println("failed to create or patch deployment")
			return ctrl.Result{}, err
		}
	}

	fmt.Println("deployment patched or created")

	//PATCH the changes
	//------------------------------------------------------
	_, _, err = kmc.PatchStatus(ctx, r.Client, bookServer, func(obj client.Object) client.Object {
		in := obj.(*apiserverv1alpha1.BookServer)
		in.Status.AvailableReplicas = bookServer.Spec.Replicas
		in.Status.Phase = apiserverv1alpha1.BookServerPending

		return in
	})
	if err != nil {
		klog.Error("failed to patch the status")
		fmt.Println("failed to patch the status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BookServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiserverv1alpha1.BookServer{}).
		Watches(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
			OwnerType:    &apiserverv1alpha1.BookServer{},
			IsController: false,
		}).
		Complete(r)
}

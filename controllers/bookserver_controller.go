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

func getNewName(bsName *string) *string {
	st := fmt.Sprintf("%s-%d", *bsName, getCurrentTime().Unix())
	return &st
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
	bs := &apiserverv1alpha1.BookServer{}
	if err := r.Get(ctx, key, bs); err != nil {
		klog.Error("failed to get book server")
		fmt.Println("failed to get book server")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	bs = bs.DeepCopy()

	isNeedToCreateNewChild := false
	childName := bs.Status.ChildDeployment
	childDep := &appsv1.Deployment{}

	if childName == nil {
		isNeedToCreateNewChild = true
	} else {
		if err := r.Client.Get(ctx, client.ObjectKey{Namespace: bs.Namespace, Name: *childName}, childDep); err == nil {
			childDep.DeepCopy()
		} else if client.IgnoreNotFound(err) == nil {
			isNeedToCreateNewChild = true
		} else {
			klog.Error("failed to get child deployment")
			fmt.Println("failed to get child deployment")
			return ctrl.Result{}, err
		}
	}

	if isNeedToCreateNewChild == true { // Create a new child

		if childName == nil {
			fmt.Println("child name before is nil")
		} else {
			fmt.Println("child name before: ", *childName)
		}

		//get a new child name
		childName = getNewName(&bs.Name)

		//patching the child name first
		_, _, err := kmc.PatchStatus(ctx, r.Client, bs, func(obj client.Object) client.Object {
			in := obj.(*apiserverv1alpha1.BookServer)
			in.Status.ChildDeployment = childName
			in.Status.Phase = apiserverv1alpha1.BookServerPending

			return in
		})
		if err != nil {
			fmt.Println("failed to patch the child name in book server")
			klog.Error("failed to patch the child name in book server")
			return ctrl.Result{}, err
		}

		// create new deployment
		childDep, _ = constructNewDeploymentForBookServer(bs, *childName)

		if err := r.Client.Create(ctx, childDep); err != nil {
			klog.Error("failed to create child deployment")
			fmt.Println("failed to create child deployment")
			return ctrl.Result{}, err
		}

	} else { // patch (update) existing deployment
		_, _, err := kmc.CreateOrPatch(ctx, r.Client, childDep, func(obj client.Object, createOp bool) client.Object {
			in := obj.(*appsv1.Deployment)
			in.Spec.Replicas = bs.Spec.Replicas
			return in
		})
		if err != nil {
			fmt.Println("failed to update the existing deployment")
			klog.Error("failed to update the existing deployment")
			return ctrl.Result{}, err
		}
	}

	//patch book server status
	_, _, err := kmc.CreateOrPatch(ctx, r.Client, bs, func(obj client.Object, createOp bool) client.Object {
		in := obj.(*apiserverv1alpha1.BookServer)
		in.Status.AvailableReplicas = bs.Spec.Replicas
		in.Status.Phase = apiserverv1alpha1.BookServerRunning
		return in
	})
	if err != nil {
		fmt.Println("failed to update book status")
		klog.Error("failed to update the book server")
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

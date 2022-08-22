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
	v13 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	apiserverv1alpha1 "github.com/Superm4n97/custom-controller/api/v1alpha1"
)

// BookServerReconciler reconciles a BookServer object
type BookServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func intToPointer(a int32) *int32 {
	return &a
}

func getCurrentTime() time.Time {
	return time.Now()
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

	klog.Info("Get event for BooksServer name: ", key.Name, " in namespace: ", key.Namespace)

	var bookServer apiserverv1alpha1.BookServer
	if err := r.Get(ctx, req.NamespacedName, &bookServer); err != nil {
		fmt.Println("unable to fetch the book server")
		klog.Error(err, "unable to fetch the book server")

		return ctrl.Result{}, err
	}

	getOwnerReference := func(bs *apiserverv1alpha1.BookServer) []metav1.OwnerReference {
		return []metav1.OwnerReference{
			{
				APIVersion: bs.APIVersion,
				Kind:       bs.Kind,
				Name:       bs.Name,
				UID:        bs.UID,
			},
		}
	}

	constructNewDeploymentForBookServer := func(bs *apiserverv1alpha1.BookServer) (*v13.Deployment, error) {

		depName := fmt.Sprintf("%s-%d", bs.Name, getCurrentTime().Unix())
		fmt.Println(depName)
		newDep := v13.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:            depName,
				Labels:          bs.Labels,
				Namespace:       bs.Namespace,
				OwnerReferences: getOwnerReference(bs),
			},
			Spec: v13.DeploymentSpec{
				Selector: &bs.Spec.Selector,
				Replicas: bs.Spec.Replicas,

				Template: v12.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: bs.Spec.Selector.MatchLabels,
					},
					Spec: v12.PodSpec{
						Containers: []v12.Container{
							{
								Name:  "bookserver",
								Image: "superm4n/book-api-server:v0.1.3",
								Ports: []v12.ContainerPort{
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

	//dlst gets the deployment that created by this book server
	/*
		var dlst = v13.Deployment{}
		if err := r.Get(ctx,namespaceName of the deployment,dlst); err != nil {
			fmt.Println("No deployment found owing by this book server")
		} else {
			fmt.Println("deployment found with size of ", dlst.Size())
		}
	*/

	fmt.Println(bookServer.Status.AvailableReplicas)

	if bookServer.Status.ChildDeployment == nil {
		dep, err := constructNewDeploymentForBookServer(&bookServer)
		if err != nil {
			klog.Error(err, "unable to fetch deployment")
			fmt.Println("unable to fetch deployment")
			return ctrl.Result{}, err
		}

		if err := r.Client.Create(ctx, dep); err != nil {
			klog.Error(err, "unable to create deployment")
			fmt.Println("unable to create deployment")
			return ctrl.Result{}, err
		}
		bookServer.Status.ChildDeployment = &dep.Name
	}

	fmt.Println("deployment created....")

	fmt.Println("Spec Replicas: ", *bookServer.Spec.Replicas)
	if bookServer.Status.AvailableReplicas == nil {
		bookServer.Status.AvailableReplicas = bookServer.Spec.Replicas
	}

	fmt.Println("Status Replicas: ", *bookServer.Status.AvailableReplicas)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BookServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiserverv1alpha1.BookServer{}).
		Complete(r)
}

// This file has been modified to enable service account token generation
// using the RBAC v1 API. Credit to the original authors at Rancher.
// https://github.com/rancher/kontainer-engine/blob/release/v2.4/drivers/util/utils.go

package main

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	cattleNamespace           = "cattle-system"
	clusterAdmin              = "cluster-admin"
	kontainerEngine           = "kontainer-engine"
	newClusterRoleBindingName = "system-netes-default-clusterRoleBinding"
	serviceAccountSecretName  = "cattle-secret"
)

func generateServiceAccountToken(clientset kubernetes.Interface) (string, error) {
	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cattleNamespace,
		},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", err
	}

	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: kontainerEngine,
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(cattleNamespace).Create(context.TODO(), serviceAccount, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf("error creating service account: %v", err)
	}

	adminRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterAdmin,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
			{
				NonResourceURLs: []string{"*"},
				Verbs:           []string{"*"},
			},
		},
	}
	clusterAdminRole, err := clientset.RbacV1().ClusterRoles().Get(context.TODO(), clusterAdmin, metav1.GetOptions{})
	if err != nil {
		clusterAdminRole, err = clientset.RbacV1().ClusterRoles().Create(context.TODO(), adminRole, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("error creating admin role: %v", err)
		}
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: newClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount.Name,
				Namespace: cattleNamespace,
				APIGroup:  v1.GroupName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterAdminRole.Name,
			APIGroup: rbacv1.GroupName,
		},
	}
	if _, err = clientset.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf("error creating role bindings: %v", err)
	}

	if serviceAccount, err = clientset.CoreV1().ServiceAccounts(cattleNamespace).Get(context.TODO(), serviceAccount.Name, metav1.GetOptions{}); err != nil {
		return "", fmt.Errorf("error getting service account: %v", err)
	}

	secret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccountSecretName,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": serviceAccount.Name,
			},
		},
		Type: v1.SecretTypeServiceAccountToken,
	}

	secretObj, err := clientset.CoreV1().Secrets(cattleNamespace).Create(
		context.TODO(),
		&secret,
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf(
			"failed to create secret for service account %s: %w",
			serviceAccount.Name,
			err,
		)
	}

	token, ok := secretObj.Data["token"]
	if !ok {
		return "", fmt.Errorf("failed to get token from generated secret")
	}

	return string(token), nil
}

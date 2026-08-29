// Package recovery contains the lower-layer control plane that remains usable
// when Pocket ID, VSO, or normal Butler is unavailable.
package recovery

import (
	"context"
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type TokenReviewer struct {
	k8s            kubernetes.Interface
	serviceAccount string
	namespace      string
}

func NewTokenReviewer(k8s kubernetes.Interface, namespace, serviceAccount string) *TokenReviewer {
	return &TokenReviewer{k8s: k8s, namespace: namespace, serviceAccount: serviceAccount}
}

func (r *TokenReviewer) Authorize(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", nil
	}
	review, err := r.k8s.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{Token: token, Audiences: []string{"butler-recovery"}},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("reviewing recovery token: %w", err)
	}
	want := "system:serviceaccount:" + r.namespace + ":" + r.serviceAccount
	if !review.Status.Authenticated || review.Status.User.Username != want {
		return "", nil
	}
	return review.Status.User.Username, nil
}

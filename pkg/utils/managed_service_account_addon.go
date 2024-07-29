package utils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var gvrMCH = schema.GroupVersionResource{
	Group:    "operator.open-cluster-management.io",
	Version:  "v1",
	Resource: "multiclusterhubs",
}
var gvrMCE = schema.GroupVersionResource{
	Group:    "multicluster.openshift.io",
	Version:  "v1",
	Resource: "multiclusterengines",
}

func unstructuredToManagedClusterAddon(
	u *unstructured.Unstructured,
) (*addonv1alpha1.ManagedClusterAddOn, error) {
	mca := &addonv1alpha1.ManagedClusterAddOn{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(
		u.UnstructuredContent(),
		mca,
	)
	if err != nil {
		return nil, err
	}

	return mca, nil
}

func DoesManagedServiceAccountAddonExist(
	hubClient dynamic.Interface,
	managedCluster *clusterv1.ManagedCluster,
) bool {
	_, err := GetManagedServiceAccountAddon(hubClient, managedCluster)
	if errors.IsNotFound(err) {
		return false
	}

	// NOTE: only false is trustworthy true is not
	return true
}

func IsManagedServiceAccountAddonAvailable(
	hubClient dynamic.Interface,
	managedCluster *clusterv1.ManagedCluster,
) bool {
	managedServiceAccountAddon, err := GetManagedServiceAccountAddon(hubClient, managedCluster)
	if err != nil {
		return false
	}

	for _, condition := range managedServiceAccountAddon.Status.Conditions {
		if condition.Type == addonv1alpha1.ManagedClusterAddOnConditionAvailable {
			if condition.Status == "True" {
				return true
			}
		}
	}
	return false
}

// list globally and get first mch, will return no error and no obj if not found
func GetMultiClusterHub(
	hubClient dynamic.Interface,
) (*unstructured.Unstructured, error) {
	uMCHList, err := hubClient.Resource(gvrMCH).List(context.TODO(), metav1.ListOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	if errors.IsNotFound(err) || len(uMCHList.Items) < 1 {
		// ignore not found errors
		return nil, nil
	}

	return &uMCHList.Items[0], nil
}

// GetMultiClusterEngine find first MCE in the cluster, will return error if no MCE can be found
func GetMultiClusterEngine(
	hubClient dynamic.Interface,
) (*unstructured.Unstructured, error) {
	uMCEList, err := hubClient.Resource(gvrMCE).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(uMCEList.Items) < 1 {
		// not found will return error
		return nil, fmt.Errorf("MulticlusterEngine CR not found.")
	}

	return &uMCEList.Items[0], nil
}

func EnableManagedServiceAccountFeature(hubClient dynamic.Interface) error {
	// can only modify mce to enable managedserviceaccount
	// modify mch will be rejected by admission webhook multiclusterhub.validating-webhook.open-cluster-management.io.
	mce, err := GetMultiClusterEngine(hubClient)
	if err != nil {
		return err
	}
	err = SetManagedServiceAcccount(mce, true)
	if err != nil {
		return err
	}
	_, err = hubClient.Resource(gvrMCE).Namespace("").Update(context.TODO(), mce, metav1.UpdateOptions{})
	return err
}

func SetManagedServiceAcccount(m *unstructured.Unstructured, state bool) error {
	components, ok, err := unstructured.NestedSlice(m.Object, "spec", "overrides", "components")
	if !ok {
		return fmt.Errorf("failed to get spec.components in %v", m)
	}
	if err != nil {
		return err
	}
	idx := -1
	elem := map[string]interface{}{
		"enabled": state,
		"name":    "managedserviceaccount",
	}
	for i, c := range components {
		component, ok := c.(map[string]interface{})
		if !ok {
			return fmt.Errorf("unexpected format for component %v expecting it to be a map[string]interface{}", component)
		}
		if name, ok := component["name"]; ok && name == elem["name"] {
			idx = i
		}
	}
	// modify components
	if idx < 0 {
		// append
		components = append(components, elem)
	} else {
		// update
		components[idx] = elem
	}
	err = unstructured.SetNestedSlice(m.Object, components, "spec", "overrides", "components")
	if err != nil {
		return err
	}
	return nil
}

func GetManagedServiceAccountAddon(
	hubClient dynamic.Interface,
	managedCluster *clusterv1.ManagedCluster,
) (*addonv1alpha1.ManagedClusterAddOn, error) {
	gvr := schema.GroupVersionResource{
		Group:    "addon.open-cluster-management.io",
		Version:  "v1alpha1",
		Resource: "managedclusteraddons",
	}

	uManagedServiceAccountAddon, err := hubClient.Resource(gvr).Namespace(managedCluster.Name).
		Get(context.TODO(), "managed-serviceaccount", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	managedServiceAccountAddon, err := unstructuredToManagedClusterAddon(uManagedServiceAccountAddon)
	if err != nil {
		return nil, err
	}

	return managedServiceAccountAddon, nil
}

func CreateManagedServiceAccountAddon(
	hubClient dynamic.Interface,
	managedCluster *clusterv1.ManagedCluster,
) (*addonv1alpha1.ManagedClusterAddOn, error) {
	gvr := schema.GroupVersionResource{
		Group:    "addon.open-cluster-management.io",
		Version:  "v1alpha1",
		Resource: "managedclusteraddons",
	}

	managedServiceAccountAddon, err := GetManagedServiceAccountAddon(hubClient, managedCluster)
	if errors.IsNotFound(err) {
		newManagedServiceAccountAddon := &addonv1alpha1.ManagedClusterAddOn{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ManagedClusterAddOn",
				APIVersion: "addon.open-cluster-management.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "managed-serviceaccount",
				Namespace: managedCluster.Name,
			},
			Spec: addonv1alpha1.ManagedClusterAddOnSpec{
				InstallNamespace: "open-cluster-management-managed-serviceaccount",
			},
		}

		uNewManagedServiceAccountAddon, err := toUnstructured(newManagedServiceAccountAddon)
		if err != nil {
			return nil, err
		}

		uManagedServiceAccountAddon, err := hubClient.
			Resource(gvr).
			Namespace(managedCluster.Name).
			Create(
				context.TODO(),
				uNewManagedServiceAccountAddon,
				metav1.CreateOptions{},
			)

		if err != nil {
			return nil, err
		}

		managedServiceAccountAddon, err := unstructuredToManagedClusterAddon(uManagedServiceAccountAddon)
		if err != nil {
			return nil, err
		}

		return managedServiceAccountAddon, nil
	}

	return managedServiceAccountAddon, nil
}

func DeleteManagedServiceAccountAddon(
	hubClient dynamic.Interface,
	managedCluster *clusterv1.ManagedCluster,
) error {
	gvr := schema.GroupVersionResource{
		Group:    "addon.open-cluster-management.io",
		Version:  "v1alpha1",
		Resource: "managedclusteraddons",
	}

	err := hubClient.Resource(gvr).Namespace(managedCluster.Name).Delete(
		context.TODO(),
		"managed-serviceaccount",
		metav1.DeleteOptions{},
	)

	return err
}

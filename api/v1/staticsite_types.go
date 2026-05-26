package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// define specifications of the desired state of the static site
type StaticSiteSpec struct {
	Replicas int32  `json:"replicas"`
	Content  string `json:"content"`          // hold raw html file
	Domain   string `json:"domain,omitempty"` // url for the web address
	// omitempty : the field is optional
}

// define the feedback structure
// Define the actual state of the static site
type StaticSiteStatus struct {
	AvailableReplicas int32 `json:"availableReplicas"`
}

// code generation tags - Kubebuilder markers ( the compiler instructions)
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// combining all together
type StaticSite struct {
	metav1.TypeMeta   `json:",inline"`            // generate the apiVersion and kind fields in the CRD YAML file
	metav1.ObjectMeta `json:"metadata,omitempty"` // generate the standard metadata block (name, namespace)

	Spec   StaticSiteSpec   `json:"spec,omitempty"`   // Enable the custom spec rules
	Status StaticSiteStatus `json:"status,omitempty"` // Enable the status tracking rules
}

//+kubebuilder:object:root=true

// StaticSiteList contains a list of StaticSite
type StaticSiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StaticSite `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StaticSite{}, &StaticSiteList{})
}

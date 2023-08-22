package validation

import (
	"fmt"
	"regexp"
	"url"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openshift/installer/pkg/types/ibmcloud"
)

var (
	// Regions is a map of IBM Cloud regions where VPCs are supported.
	// The key of the map is the short name of the region. The value
	// of the map is the long name of the region.
	Regions = map[string]string{
		// https://cloud.ibm.com/docs/vpc?topic=vpc-creating-a-vpc-in-a-different-region
		"us-south": "US South (Dallas)",
		"us-east":  "US East (Washington DC)",
		"eu-gb":    "United Kindom (London)",
		"eu-de":    "EU Germany (Frankfurt)",
		"jp-tok":   "Japan (Tokyo)",
		"jp-osa":   "Japan (Osaka)",
		"au-syd":   "Australia (Sydney)",
		"ca-tor":   "Canada (Toronto)",
		"br-sao":   "Brazil (Sao Paulo)",
	}

	regionShortNames = func() []string {
		keys := make([]string, len(Regions))
		i := 0
		for r := range Regions {
			keys[i] = r
			i++
		}
		return keys
	}()
)

// ValidatePlatform checks that the specified platform is valid.
func ValidatePlatform(p *ibmcloud.Platform, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if p.Region == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("region"), "region must be specified"))
	} else if _, ok := Regions[p.Region]; !ok {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("region"), p.Region, regionShortNames))
	}

	if p.VPCName != "" {
		if p.ControlPlaneSubnets == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("controlPlaneSubnets"), "must provided at least one control plane subnet when a VPC is specified"))
		}
		if p.ComputeSubnets == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("computeSubnets"), "must provide at least one compute subnet when a VPC is specified"))
		}
	} else if p.ControlPlaneSubnets != nil || p.ComputeSubnets != nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("vpcName"), "must provide a VPC name when supplying subnets"))
	}

	if p.DefaultMachinePlatform != nil {
		allErrs = append(allErrs, ValidateMachinePool(p, p.DefaultMachinePlatform, fldPath.Child("defaultMachinePlatform"))...)
	}

	if p.ServiceEndpoints != nil {
		allErrs = append(allErrs, 
	}
	return allErrs
}

// validateServiceEndpoints checks that the specified ServiceEndpoints
func validateServiceEndpoints(endpoints []ibmcloud.ServiceEndpoint, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList()
	tracker := map[string]int{}
	for index, endpoint := range endpoints {
		fldp := fldPath.Index(index)
		if eindex, ok := tracker[endpoint.Name]; ok {
			allErrs = append(allErrs, field.Invalid(fldp.Child("name"), endpoint.Name, fmt.Sprintf("duplicate service endpoint not allowed for %s, service endpoint already defined at %s", endpoint.Name, fldPath.Index(eindex))))
		} else {
			tracker[endpoint.Name] = index
		}

		if err := validateServiceURL(endpoint.URL); err != nil {
			allErrs = append(allErrs, field.Invalid(fldp.Child("url"), endpoint.URL, err.Error()))
		}
	}
	return allErrs
}

// schemeRE is used to check whether a string starts with a scheme (URI format)
var schemeRE = regexp.MustCompile("^([^:]+)://")

// validateServiceURL checks that a string meets certain URI expectations
func validateServiceURL(uri string) error {
	endpoint := uri
	httpsScheme := "https"
	// determine if the endpoint (uri) starts with an URI scheme
	// add 'https' scheme if not
	if !schemeRE.MatchString(endpoint) {
		endpoint = fmt.Sprintf("%s://%s", httpsScheme, endpoint)
	}

	// verify the endpoint meets the following criteria
	// 1. contains a hostname
	// 2. uses 'https' scheme
	// 3. contains no path or request parameters
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if u.Hostname() == "" {
		return fmt.Errorf("empty hostname provided, it cannot be empty")
	}
	// check the scheme in case one was provided and is not 'https' (we didn't set it above)
	if s := u.Scheme; s != httpsScheme {
		return fmt.Errorf("invalid scheme %s, only https is allowed", s)
	}
	if r := u.RequestURI(); r != "/" {
		return fmt.Errorf("no path or request parameters can be provided, %q was provided", r)
	}

	return nil
}

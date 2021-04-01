package dag

import (
	"fmt"
	"sort"
	"strings"

	ingressroutev1 "github.com/projectcontour/contour/apis/contour/v1beta1"
	"github.com/projectcontour/contour/internal/annotation"
)

// validIngressRoutesAdobe ensures the extra vhost headers don't conflict
// with other IngressRoutes vhosts or fqdns
func (b *Builder) validIngressRoutesAdobe() []*ingressroutev1.IngressRoute {
	valid := b.validIngressRoutes()

	// list of all vhost headers
	hostIngressroutes := make(map[string][]*ingressroutev1.IngressRoute)
	for _, ir := range valid {
		if vhosts := annotation.ExtraVHosts(ir); vhosts != nil {
			for _, vh := range vhosts {
				hostIngressroutes[vh] = append(hostIngressroutes[vh], ir)
			}
		}
	}

	// no extra vhosts, no extra validation needed
	if len(hostIngressroutes) == 0 {
		return valid
	}

	// list of all fqdns
	fqdnIngressroutes := make(map[string][]*ingressroutev1.IngressRoute)
	for _, ir := range valid {
		if ir.Spec.VirtualHost != nil {
			fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn] = append(fqdnIngressroutes[ir.Spec.VirtualHost.Fqdn], ir)
		}
	}

	// find the invalid ones
	//   A) empty vhost
	//   B) invalid format
	//   C) vhost against other vhosts
	//   D) vhost against other fqdns
	//   E) fqdn against other vhosts
	// (fqdn against other fqdns is done by upstream)
	var invalid = make(map[*ingressroutev1.IngressRoute]string)
	for vh, irs := range hostIngressroutes {
		// == A)
		if strings.Trim(vh, " ") == "" {
			for _, ir := range irs {
				invalid[ir] = "empty value in host annotation"
			}
			continue
		}
		// == B)
		// Don't allow '*' at all, which takes care of the suffix ':*'
		if strings.Contains(vh, "*") {
			for _, ir := range irs {
				invalid[ir] = "illegal charaters in host annotation"
			}
			continue
		}
		// == C)
		if len(irs) > 1 {
			var conflicting []string
			for _, ir := range irs {
				conflicting = append(conflicting, ir.Namespace+"/"+ir.Name)
			}
			sort.Strings(conflicting)
			msg := fmt.Sprintf("host annotation %q is duplicated in multiple IngressRoutes: %s", vh, strings.Join(conflicting, ", "))
			for _, ir := range irs {
				invalid[ir] = msg
			}
			continue
		}
		// == D)
		if firs, ok := fqdnIngressroutes[vh]; ok {
			var conflicting3 []string
			for _, ir := range firs {
				conflicting3 = append(conflicting3, ir.Namespace+"/"+ir.Name)
			}
			sort.Strings(conflicting3)
			msg := fmt.Sprintf("host annotation %q is duplicated with a fqdn in other IngressRoutes: %s", vh, strings.Join(conflicting3, ", "))
			for _, ir := range irs {
				invalid[ir] = msg
			}
		}
	}
	for fqdn, irs := range fqdnIngressroutes {
		// if we already have an issue with this IngressRoute, skip
		if _, seen := invalid[irs[0]]; seen {
			continue
		}
		// == E)
		if hirs, ok := hostIngressroutes[fqdn]; ok {
			var conflicting3 []string
			for _, ir := range hirs {
				conflicting3 = append(conflicting3, ir.Namespace+"/"+ir.Name)
			}
			sort.Strings(conflicting3)
			msg := fmt.Sprintf("fqdn %q is duplicated with a host annotation in other IngressRoutes: %s", fqdn, strings.Join(conflicting3, ", "))
			for _, ir := range irs {
				invalid[ir] = msg
			}
		}
	}

	if len(invalid) == 0 {
		return valid
	}

	var valid2 []*ingressroutev1.IngressRoute
	for _, ir := range valid {
		msg, ok := invalid[ir]
		if !ok {
			valid2 = append(valid2, ir)
			continue
		}
		sw, commit := b.WithObject(ir)
		sw.SetInvalid(msg)
		commit()
	}

	return valid2
}

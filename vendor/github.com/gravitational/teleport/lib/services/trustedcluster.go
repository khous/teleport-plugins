/*
Copyright 2021 Gravitational, Inc.

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

package services

import (
	"fmt"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/lib/modules"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
)

// ValidateTrustedCluster checks and sets Trusted Cluster defaults
func ValidateTrustedCluster(tc TrustedCluster) error {
	if err := tc.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}

	// we are not mentioning Roles parameter because we are deprecating it
	if len(tc.GetRoles()) == 0 && len(tc.GetRoleMap()) == 0 {
		if err := modules.GetModules().EmptyRolesHandler(); err != nil {
			return trace.Wrap(err)
		}
		// OSS teleport uses 'admin' by default:
		tc.SetRoleMap(RoleMap{
			RoleMapping{
				Remote: teleport.AdminRoleName,
				Local:  []string{teleport.AdminRoleName},
			},
		})
	}

	if _, err := parseRoleMap(tc.GetRoleMap()); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// RoleMapToString prints user friendly representation of role mapping
func RoleMapToString(r RoleMap) string {
	values, err := parseRoleMap(r)
	if err != nil {
		return fmt.Sprintf("<failed to parse: %v", err)
	}
	if len(values) != 0 {
		return fmt.Sprintf("%v", values)
	}
	return "<empty>"
}

func parseRoleMap(r RoleMap) (map[string][]string, error) {
	directMatch := make(map[string][]string)
	for i := range r {
		roleMap := r[i]
		if roleMap.Remote == "" {
			return nil, trace.BadParameter("missing 'remote' parameter for role_map")
		}
		_, err := utils.ReplaceRegexp(roleMap.Remote, "", "")
		if trace.IsBadParameter(err) {
			return nil, trace.BadParameter("failed to parse 'remote' parameter for role_map: %v", err.Error())
		}
		if len(roleMap.Local) == 0 {
			return nil, trace.BadParameter("missing 'local' parameter for 'role_map'")
		}
		for _, local := range roleMap.Local {
			if local == "" {
				return nil, trace.BadParameter("missing 'local' property of 'role_map' entry")
			}
			if local == Wildcard {
				return nil, trace.BadParameter("wildcard value is not supported for 'local' property of 'role_map' entry")
			}
		}
		_, ok := directMatch[roleMap.Remote]
		if ok {
			return nil, trace.BadParameter("remote role '%v' match is already specified", roleMap.Remote)
		}
		directMatch[roleMap.Remote] = roleMap.Local
	}
	return directMatch, nil
}

// MapRoles maps local roles to remote roles
func MapRoles(r RoleMap, remoteRoles []string) ([]string, error) {
	_, err := parseRoleMap(r)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var outRoles []string
	// when no remote roles are specified, assume that
	// there is a single empty remote role (that should match wildcards)
	if len(remoteRoles) == 0 {
		remoteRoles = []string{""}
	}
	for _, mapping := range r {
		expression := mapping.Remote
		for _, remoteRole := range remoteRoles {
			// never map default implicit role, it is always
			// added by default
			if remoteRole == constants.DefaultImplicitRole {
				continue
			}
			for _, replacementRole := range mapping.Local {
				replacement, err := utils.ReplaceRegexp(expression, replacementRole, remoteRole)
				switch {
				case err == nil:
					// empty replacement can occur when $2 expand refers
					// to non-existing capture group in match expression
					if replacement != "" {
						outRoles = append(outRoles, replacement)
					}
				case trace.IsNotFound(err):
					continue
				default:
					return nil, trace.Wrap(err)
				}
			}
		}
	}
	return outRoles, nil
}

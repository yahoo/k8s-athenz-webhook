package webhook

import (
	"log"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/ardielle/ardielle-go/rdl"
	"github.com/yahoo/athenz/clients/go/zms"
	v1 "github.com/yahoo/k8s-athenz-istio-auth/pkg/apis/athenz/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	domainName = "home.domain"
	username   = "user.name"
)

func getFakeAthenzDomains() *v1.AthenzDomain {
	spec := v1.AthenzDomainSpec{
		SignedDomain: getFakeDomain(),
	}
	item := &v1.AthenzDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: domainName,
		},
		Spec: spec,
	}
	return item
}

func getFakeDomain() zms.SignedDomain {
	allow := zms.ALLOW
	timestamp, err := rdl.TimestampParse("2019-06-21T19:28:09.305Z")
	if err != nil {
		panic(err)
	}

	return zms.SignedDomain{
		Domain: &zms.DomainData{
			Modified: timestamp,
			Name:     zms.DomainName(domainName),
			Policies: &zms.SignedPolicies{
				Contents: &zms.DomainPolicies{
					Domain: zms.DomainName(domainName),
					Policies: []*zms.Policy{
						{
							Assertions: []*zms.Assertion{
								{
									Role:     domainName + ":role.admin",
									Resource: domainName + ":*",
									Action:   "*",
									Effect:   &allow,
								},
							},
							Modified: &timestamp,
							Name:     zms.ResourceName(domainName + ":policy.admin"),
						},
					},
				},
				KeyId:     "col-env-1.1",
				Signature: "signature-policy",
			},
			Roles: []*zms.Role{
				{
					Members:  []zms.MemberName{zms.MemberName(username)},
					Modified: &timestamp,
					Name:     zms.ResourceName(domainName + ":role.admin"),
					RoleMembers: []*zms.RoleMember{
						{
							MemberName: zms.MemberName(username),
						},
					},
				},
				{
					Name:  zms.ResourceName(domainName + ":role.delegated"),
					Trust: "test.delegated.domain",
				},
			},
			Services: []*zms.ServiceIdentity{},
			Entities: []*zms.Entity{},
		},
		KeyId:     "colo-env-1.1",
		Signature: "signature",
	}
}

func newCache() *Cache {
	domainMap := make(map[string]roleMappings)
	c := &Cache{
		domainMap: domainMap,
		log:       log.New(os.Stderr, "", log.LstdFlags),
	}
	roleToPrincipals := make(map[string][]*simplePrincipal)
	roleToAssertion := make(map[string][]*simpleAssertion)
	crMap := roleMappings{
		roleToPrincipals: roleToPrincipals,
		roleToAssertion:  roleToAssertion,
	}
	c.domainMap[domainName] = crMap
	return c
}

func TestParseData(t *testing.T) {
	c := newCache()
	item := getFakeAthenzDomains()
	crMap, err := c.parseData(item)
	if err != nil {
		t.Error(err)
	}
	if len(crMap.roleToPrincipals) != 1 || crMap.roleToPrincipals["home.domain:role.admin"] == nil {
		t.Error("Failed to create RoleToPrincipals map")
	}

	if len(crMap.roleToAssertion) != 1 || crMap.roleToPrincipals["home.domain:role.admin"] == nil {
		t.Error("Failed to create RoleToAssertion map")
	}
}

func TestParseDataNilCase(t *testing.T) {
	c := newCache()
	item := getFakeAthenzDomains()
	item.Spec.SignedDomain.Domain.Policies.Contents = nil
	_, err := c.parseData(item)
	if err.Error() != "One of AthenzDomain, Domain field in SignedDomain, Domain Policies field or Policies Contents is nil" {
		t.Error("did not catch policies content nil")
	}

	item.Spec.SignedDomain.Domain.Policies = nil
	_, err = c.parseData(item)
	if err.Error() != "One of AthenzDomain, Domain field in SignedDomain, Domain Policies field or Policies Contents is nil" {
		t.Error("did not catch policies nil")
	}

	item.Spec.SignedDomain.Domain = nil
	_, err = c.parseData(item)
	if err.Error() != "One of AthenzDomain, Domain field in SignedDomain, Domain Policies field or Policies Contents is nil" {
		t.Error("did not catch Domain data nil")
	}

	_, err = c.parseData(nil)
	if err.Error() != "One of AthenzDomain, Domain field in SignedDomain, Domain Policies field or Policies Contents is nil" {
		t.Error("did not catch item nil")
	}
}

func TestParseDataPrincipal(t *testing.T) {
	c := newCache()
	item := getFakeAthenzDomains()

	// role has nil field or empty object
	item.Spec.SignedDomain.Domain.Roles = []*zms.Role{
		{},
		nil,
	}
	crmap, err := c.parseData(item)
	if err != nil {
		t.Error(err)
	}
	if len(crmap.roleToPrincipals) != 0 {
		t.Error("roleToPrincipal map should be empty since roles are empty or nil")
	}

	// roleMember is nil or roleMember name is nil
	item.Spec.SignedDomain.Domain.Roles = []*zms.Role{
		{
			Members: []zms.MemberName{zms.MemberName(username)},
			Name:    zms.ResourceName(domainName + ":role.admin"),
			RoleMembers: []*zms.RoleMember{
				{
					MemberName: zms.MemberName(""),
				},
			},
		},
		{
			Members: []zms.MemberName{zms.MemberName(username)},
			Name:    zms.ResourceName(domainName + ":role.admin"),
			RoleMembers: []*zms.RoleMember{
				{},
				nil,
			},
		},
	}
	crmap, err = c.parseData(item)
	if err != nil {
		t.Error(err)
	}
	if len(crmap.roleToPrincipals["home.domain:role.admin"]) != 0 {
		t.Error("roleToPrincipal array should be empty since role members are empty")
	}

	// regex conversion fail
	item.Spec.SignedDomain.Domain.Roles = []*zms.Role{
		{
			Members: []zms.MemberName{zms.MemberName(username)},
			Name:    zms.ResourceName(domainName + ":role.admin"),
			RoleMembers: []*zms.RoleMember{
				{
					MemberName: zms.MemberName("/?([a-zA-Z0-9_+-\\s+]+)"),
				},
			},
		},
	}
	crmap, err = c.parseData(item)
	if err != nil {
		t.Error(err)
	}
	if len(crmap.roleToPrincipals["home.domain:role.admin"]) != 0 {
		t.Error("member shouldn't be added to the map because member name regex is invalid")
	}
}

func TestParseDataPolicy(t *testing.T) {
	c := newCache()
	item := getFakeAthenzDomains()

	// policy is nil
	item.Spec.SignedDomain.Domain.Policies.Contents.Policies = []*zms.Policy{
		{},
		nil,
	}
	crmap, err := c.parseData(item)
	if err != nil {
		t.Error(err)
	}
	if len(crmap.roleToAssertion) != 0 {
		t.Error("map entries shouldn't be added because policies are nil or empty")
	}

	// assertion is nil
	item.Spec.SignedDomain.Domain.Policies.Contents.Policies = []*zms.Policy{
		{
			Assertions: []*zms.Assertion{
				{},
				nil,
			},
			Name: zms.ResourceName(domainName + ":policy.admin"),
		},
	}
	crmap, err = c.parseData(item)
	if err != nil {
		t.Error(err)
	}
	if len(crmap.roleToAssertion) != 0 {
		t.Error("map entries shouldn't be added because assertions are nil or empty")
	}
}

func TestAddOrUpdateObj(t *testing.T) {
	c := newCache()
	item := getFakeAthenzDomains()

	// add athenz domain
	c.addOrUpdateObj(item)
	obj, ok := c.domainMap[domainName]
	if !ok {
		t.Error("Failed to add AthenzDomain to domainMap")
	}
	if len(obj.roleToPrincipals["home.domain:role.admin"]) != 1 {
		t.Error("Failed to add AthenzDomain to domainMap. RoleToPrincipals is empty.")
	}
	targetRegex, err := regexp.Compile("^user.name$")
	if err != nil {
		t.Error(err)
	}
	actualRegex := obj.roleToPrincipals["home.domain:role.admin"][0].memberRegex
	if actualRegex.String() != targetRegex.String() {
		t.Error("member added to the map does not match target regex")
	}

	// update athenz domains
	item.Spec.Domain.Roles = []*zms.Role{
		{
			Members: []zms.MemberName{zms.MemberName(username)},
			Name:    zms.ResourceName(domainName + ":role.admin.test1"),
			RoleMembers: []*zms.RoleMember{
				{
					MemberName: zms.MemberName(username),
					Expiration: &rdl.Timestamp{
						Time: time.Now().Add(time.Hour),
					},
				},
			},
		},
		{
			Members: []zms.MemberName{zms.MemberName(username + "1")},
			Name:    zms.ResourceName(domainName + ":role.admin.test2"),
			RoleMembers: []*zms.RoleMember{
				{
					MemberName: zms.MemberName(username),
					Expiration: &rdl.Timestamp{
						Time: time.Now().Add(2 * time.Hour),
					},
				},
			},
		},
	}
	c.addOrUpdateObj(item)
	crMap, ok := c.domainMap[domainName]
	if !ok {
		t.Error("Failed to keep AthenzDomain to domainMap")
	}
	if len(crMap.roleToPrincipals) != 2 {
		t.Error("Failed to update AthenzDomain roles")
	}
	if crMap.roleToPrincipals["home.domain:role.admin.test1"][0].memberRegex.String() != targetRegex.String() {
		t.Error("Unable to find correct member for role home.domain:role.admin.test1")
	}
	if crMap.roleToPrincipals["home.domain:role.admin.test2"][0].memberRegex.String() != targetRegex.String() {
		t.Error("Unable to find correct member for role home.domain:role.admin.test2")
	}
}

func TestDeleteObj(t *testing.T) {
	c := newCache()
	item := getFakeAthenzDomains()
	c.deleteObj(item)
	_, ok := c.domainMap[domainName]
	if ok {
		t.Error("Failed to delete AthenzDomain in domainMap")
	}
}

func TestAuthorize(t *testing.T) {
	privateCache := newCache()
	item := getFakeAthenzDomains()
	crMap, err := privateCache.parseData(item)
	if err != nil {
		t.Error(err)
	}
	privateCache.domainMap[domainName] = crMap

	// grant access
	check := AthenzAccessCheck{
		Action:   "get",
		Resource: "home.domain:pods",
	}
	res, err := privateCache.authorize(username, check)
	if err != nil {
		t.Error(err)
	}
	if !res {
		t.Error("Wrong authorization result, authorization should pass.")
	}

	// deny access
	check = AthenzAccessCheck{
		Action:   "get",
		Resource: "home.domain:pods",
	}
	res, err = privateCache.authorize("fakeclient", check)
	if res {
		t.Error("Wrong authorization result, fakeclient's request should be denied")
	}

	// check resource does not exist in cache
	check = AthenzAccessCheck{
		Action:   "get",
		Resource: "home.domain.test:pods",
	}
	res, err = privateCache.authorize(username, check)
	if err.Error() != "home.domain.test does not exist in cache map" {
		t.Error("should throw an error when domain does not exist in map")
	}

	// expired membership
	check = AthenzAccessCheck{
		Action:   "get",
		Resource: "home.domain:pods",
	}
	item.Spec.Domain.Roles = []*zms.Role{
		{
			Members: []zms.MemberName{zms.MemberName(username)},
			Name:    zms.ResourceName(domainName + ":role.admin"),
			RoleMembers: []*zms.RoleMember{
				{
					MemberName: zms.MemberName(username + "1"),
					Expiration: &rdl.Timestamp{
						Time: time.Now().Add(time.Duration(-10) * time.Hour),
					},
				},
			},
		},
	}
	privateCache.addOrUpdateObj(item)
	res, err = privateCache.authorize(username+"1", check)
	if err != nil {
		t.Error(err)
	}
	if res {
		t.Error("Wrong authorization result. Membership has expired")
	}
}
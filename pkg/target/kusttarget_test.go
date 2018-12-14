/*
Copyright 2018 The Kubernetes Authors.

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

package target

import (
	"encoding/base64"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/k8sdeps/transformer"
	"sigs.k8s.io/kustomize/pkg/constants"
	"sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/kustomize/pkg/gvk"
	"sigs.k8s.io/kustomize/pkg/ifc"
	"sigs.k8s.io/kustomize/pkg/internal/loadertest"
	"sigs.k8s.io/kustomize/pkg/resid"
	"sigs.k8s.io/kustomize/pkg/resmap"
	"sigs.k8s.io/kustomize/pkg/resource"
	"sigs.k8s.io/kustomize/pkg/types"
)

const (
	kustomizationContent1 = `
namePrefix: foo-
nameSuffix: -bar
namespace: ns1
commonLabels:
  app: nginx
commonAnnotations:
  note: This is a test annotation
resources:
  - deployment.yaml
  - namespace.yaml
configMapGenerator:
- name: literalConfigMap
  literals:
  - DB_USERNAME=admin
  - DB_PASSWORD=somepw
secretGenerator:
- name: secret
  commands:
    DB_USERNAME: "printf admin"
    DB_PASSWORD: "printf somepw"
  type: Opaque
patchesJson6902:
- target:
    group: apps
    version: v1
    kind: Deployment
    name: dply1
  path: jsonpatch.json
`
	kustomizationContent2 = `
secretGenerator:
- name: secret
  timeoutSeconds: 1
  commands:
    USER: "sleep 2"
  type: Opaque
`
	deploymentContent = `apiVersion: apps/v1
metadata:
  name: dply1
kind: Deployment
`
	namespaceContent = `apiVersion: v1
kind: Namespace
metadata:
  name: ns1
`
	jsonpatchContent = `[
    {"op": "add", "path": "/spec/replica", "value": "3"}
]`
)

var rf = resmap.NewFactory(resource.NewFactory(
	kunstruct.NewKunstructuredFactoryImpl()))

func makeKustTarget(t *testing.T, l ifc.Loader) *KustTarget {
	fakeFs := fs.MakeFakeFS()
	fakeFs.Mkdir("/")
	kt, err := NewKustTarget(
		l, fakeFs, rf, transformer.NewFactoryImpl())
	if err != nil {
		t.Fatalf("Unexpected construction error %v", err)
	}
	return kt
}

func makeLoader1(t *testing.T) ifc.Loader {
	ldr := loadertest.NewFakeLoader("/testpath")
	err := ldr.AddFile("/testpath/"+constants.KustomizationFileName, []byte(kustomizationContent1))
	if err != nil {
		t.Fatalf("Failed to setup fake ldr.")
	}
	err = ldr.AddFile("/testpath/deployment.yaml", []byte(deploymentContent))
	if err != nil {
		t.Fatalf("Failed to setup fake ldr.")
	}
	err = ldr.AddFile("/testpath/namespace.yaml", []byte(namespaceContent))
	if err != nil {
		t.Fatalf("Failed to setup fake ldr.")
	}
	err = ldr.AddFile("/testpath/jsonpatch.json", []byte(jsonpatchContent))
	if err != nil {
		t.Fatalf("Failed to setup fake ldr.")
	}
	return ldr
}

var deploy = gvk.Gvk{Group: "apps", Version: "v1", Kind: "Deployment"}
var cmap = gvk.Gvk{Version: "v1", Kind: "ConfigMap"}
var secret = gvk.Gvk{Version: "v1", Kind: "Secret"}
var ns = gvk.Gvk{Version: "v1", Kind: "Namespace"}

func TestResources1(t *testing.T) {
	expected := resmap.ResMap{
		resid.NewResIdWithPrefixSuffixNamespace(deploy, "dply1", "foo-", "-bar", "ns1"): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "foo-dply1-bar",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"spec": map[string]interface{}{
					"replica": "3",
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": "nginx",
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"note": "This is a test annotation",
							},
							"labels": map[string]interface{}{
								"app": "nginx",
							},
						},
					},
				},
			}),
		resid.NewResIdWithPrefixSuffixNamespace(cmap, "literalConfigMap", "foo-", "-bar", "ns1"): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "foo-literalConfigMap-bar-8d2dkb8k24",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"data": map[string]interface{}{
					"DB_USERNAME": "admin",
					"DB_PASSWORD": "somepw",
				},
			}).SetBehavior(ifc.BehaviorCreate),
		resid.NewResIdWithPrefixSuffixNamespace(secret, "secret", "foo-", "-bar", "ns1"): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "foo-secret-bar-9btc7bt4kb",
					"namespace": "ns1",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
				"type": ifc.SecretTypeOpaque,
				"data": map[string]interface{}{
					"DB_USERNAME": base64.StdEncoding.EncodeToString([]byte("admin")),
					"DB_PASSWORD": base64.StdEncoding.EncodeToString([]byte("somepw")),
				},
			}).SetBehavior(ifc.BehaviorCreate),
		resid.NewResIdWithPrefixSuffixNamespace(ns, "ns1", "foo-", "-bar", ""): rf.RF().FromMap(
			map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "foo-ns1-bar",
					"labels": map[string]interface{}{
						"app": "nginx",
					},
					"annotations": map[string]interface{}{
						"note": "This is a test annotation",
					},
				},
			}),
	}
	actual, err := makeKustTarget(
		t, makeLoader1(t)).MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("unexpected Resources error %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		err = expected.ErrorIfNotEqual(actual)
		t.Fatalf("unexpected inequality: %v", err)
	}
}

func TestResourceNotFound(t *testing.T) {
	l := loadertest.NewFakeLoader("/testpath")
	err := l.AddFile("/testpath/"+constants.KustomizationFileName, []byte(kustomizationContent1))
	if err != nil {
		t.Fatalf("Failed to setup fake ldr.")
	}
	_, err = makeKustTarget(t, l).MakeCustomizedResMap()
	if err == nil {
		t.Fatalf("Didn't get the expected error for an unknown resource")
	}
	if !strings.Contains(err.Error(), `cannot read file "/testpath/deployment.yaml"`) {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestSecretTimeout(t *testing.T) {
	l := loadertest.NewFakeLoader("/testpath")
	err := l.AddFile("/testpath/"+constants.KustomizationFileName, []byte(kustomizationContent2))
	if err != nil {
		t.Fatalf("Failed to setup fake ldr.")
	}
	_, err = makeKustTarget(t, l).MakeCustomizedResMap()
	if err == nil {
		t.Fatalf("Didn't get the expected error for an unknown resource")
	}
	if !strings.Contains(err.Error(), "killed") {
		t.Fatalf("unexpected error: %q", err)
	}
}

func findSecret(m resmap.ResMap) *resource.Resource {
	for id, res := range m {
		if id.Gvk().Kind == "Secret" {
			return res
		}
	}
	return nil
}

func TestDisableNameSuffixHash(t *testing.T) {
	kt := makeKustTarget(t, makeLoader1(t))

	m, err := kt.MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("unexpected Resources error %v", err)
	}
	secret := findSecret(m)
	if secret == nil {
		t.Errorf("Expected to find a Secret")
	}
	if secret.GetName() != "foo-secret-bar-9btc7bt4kb" {
		t.Errorf("unexpected secret resource name: %s", secret.GetName())
	}

	kt.kustomization.GeneratorOptions = &types.GeneratorOptions{
		DisableNameSuffixHash: true}
	m, err = kt.MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("unexpected Resources error %v", err)
	}
	secret = findSecret(m)
	if secret == nil {
		t.Errorf("Expected to find a Secret")
	}
	if secret.GetName() != "foo-secret-bar" { // No hash at end.
		t.Errorf("unexpected secret resource name: %s", secret.GetName())
	}
}

func write(t *testing.T, ldr loadertest.FakeLoader, dir string, content string) {
	err := ldr.AddFile(
		filepath.Join(dir, constants.KustomizationFileName),
		[]byte(`
apiVersion: v1
kind: Kustomization
`+content))
	if err != nil {
		t.Fatalf("Failed to setup fake loader.")
	}
}

func TestIssue596AllowDirectoriesThatAreSubstringsOfEachOther(t *testing.T) {
	ldr := loadertest.NewFakeLoader(
		"/app/overlays/aws-sandbox2.us-east-1")
	write(t, ldr, "/app/base", "")
	write(t, ldr, "/app/overlays/aws", `
bases:
- ../../base
`)
	write(t, ldr, "/app/overlays/aws-nonprod", `
bases:
- ../aws
`)
	write(t, ldr, "/app/overlays/aws-sandbox2.us-east-1", `
bases:
- ../aws-nonprod
`)
	m, err := makeKustTarget(t, ldr).MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	if m == nil {
		t.Fatalf("Empty map.")
	}
}

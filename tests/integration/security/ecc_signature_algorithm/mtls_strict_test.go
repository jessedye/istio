//go:build integ
// +build integ

//  Copyright Istio Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package eccsignaturealgorithm

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"

	"istio.io/istio/pkg/test/echo/common/scheme"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/cluster/kube"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/util/tmpl"
	"istio.io/istio/tests/integration/security/util"
	"istio.io/istio/tests/integration/security/util/cert"
)

const (
	DestinationRuleConfigIstioMutual = `
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: server
  namespace: {{.AppNamespace}}
spec:
  host: "server.{{.AppNamespace}}.svc.cluster.local"
  trafficPolicy:
    tls:
      mode: ISTIO_MUTUAL
`

	PeerAuthenticationConfig = `
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: {{.AppNamespace}}
spec:
  mtls:
    mode: STRICT
`
)

func TestStrictMTLS(t *testing.T) {
	framework.
		NewTest(t).
		Features("security.peer.ecc-signature-algorithm").
		Run(func(t framework.TestContext) {
			peerTemplate := tmpl.EvaluateOrFail(t, PeerAuthenticationConfig, map[string]string{"AppNamespace": apps.Namespace.Name()})
			t.ConfigIstio().ApplyYAMLOrFail(t, apps.Namespace.Name(), peerTemplate)
			util.WaitForConfig(t, apps.Namespace, peerTemplate)

			drTemplate := tmpl.EvaluateOrFail(t, DestinationRuleConfigIstioMutual, map[string]string{"AppNamespace": apps.Namespace.Name()})
			t.ConfigIstio().ApplyYAMLOrFail(t, apps.Namespace.Name(), drTemplate)
			util.WaitForConfig(t, apps.Namespace, drTemplate)

			response := apps.Client.CallOrFail(t, echo.CallOptions{
				Target:   apps.Server,
				PortName: "http",
				Scheme:   scheme.HTTP,
				Count:    1,
			})

			if err := response.CheckOK(); err != nil {
				t.Fatalf("client could not reach server: %v", err)
			}

			kubeconfig := (t.Clusters().Default().(*kube.Cluster)).Filename()
			target := fmt.Sprintf("server.%s:8091", apps.Namespace.Name())
			certPEM, err := cert.DumpCertFromSidecar(apps.Namespace, "app=client", "istio-proxy", kubeconfig, target)
			if err != nil {
				t.Fatalf("client could not get certificate from server: %v", err)
			}
			block, _ := pem.Decode([]byte(certPEM))
			if block == nil { // nolint: staticcheck
				t.Fatalf("failed to parse certificate PEM")
			}

			certificate, parseErr := x509.ParseCertificate(block.Bytes) // nolint: staticcheck
			if err != nil {
				t.Fatalf("failed to parse certificate: %v", parseErr)
			}

			if certificate.PublicKeyAlgorithm != x509.ECDSA {
				t.Fatalf("public key used in server cert is not ECDSA: %v", certificate.PublicKeyAlgorithm)
			}
		})
}

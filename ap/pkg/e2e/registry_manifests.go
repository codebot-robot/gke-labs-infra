// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

const RegistryManifests = `
apiVersion: v1
kind: Service
metadata:
  name: images
  namespace: default
spec:
  ports:
  - name: registry
    port: 5000
    targetPort: 5000
  - name: http
    port: 80
    targetPort: 5000
  selector:
    app: images
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: images
  namespace: default
spec:
  serviceName: images
  replicas: 1
  selector:
    matchLabels:
      app: images
  template:
    metadata:
      labels:
        app: images
    spec:
      containers:
      - name: registry
        image: registry:2
        ports:
        - containerPort: 5000
        env:
        - name: REGISTRY_HTTP_ADDR
          value: :5000
        volumeMounts:
        - name: data
          mountPath: /var/lib/registry
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 1Gi
`

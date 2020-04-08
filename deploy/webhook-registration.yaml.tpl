apiVersion: admissionregistration.k8s.io/v1beta1
kind: MutatingWebhookConfiguration
metadata:
  name: longhorn-monitor
  labels:
    app: longhorn-monitor
    kind: mutator
webhooks:
  - name: longhorn-monitor.der-fetzer.de
    clientConfig:
      service:
        name: longhorn-monitor-webhook
        namespace: longhorn-addon
        path: "/mutate"
      caBundle: CA_BUNDLE
    rules:
      - operations: [ "CREATE" ]
        apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
        
## About Openshift Route

An [OpenShift Container Platform route](https://docs.openshift.com/container-platform/3.11/architecture/networking/routes.html) exposes a [service](https://docs.openshift.com/container-platform/3.11/architecture/core_concepts/pods_and_services.html#services) at a host name, such as www.example.com, so that external clients can reach it by name.

Routes can be either secured or unsecured. Secure routes provide the ability to use several types of TLS termination to serve certificates to the client.

## How to integrate Openshift Routes with Argo Rollouts

NOTES:

**_1. The file as follows (and the codes in it) just for illustrative purposes only, please do not use directly!!!_**

**_2. The argo-rollouts >= [v1.7.0-rc1](https://github.com/argoproj/argo-rollouts/releases/tag/v1.7.0-rc1)_**

Steps:

1. Run the `yaml/rbac.yaml` to add the role for operate on the `Openshift Route`.
2. Build this plugin.
3. Put the plugin somewhere & mount on to the `argo-rollouts` container (please refer to the example YAML below to modify the deployment):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: argo-rollouts
  namespace: argo-rollouts
spec:
  template:
    spec:
      ...
      volumes:
        ...
         - name: openshift-route-plugin
           hostPath:
             path: /CHANGE-ME/rollouts-plugin-trafficrouter-openshift
             type: ''
      containers:
        - name: argo-rollouts
        ...
          volumeMounts:
             - name: openshift-route-plugin
               mountPath: /CHANGE-ME/rollouts-plugin-trafficrouter-openshift

```

4. Create a ConfigMap to let `argo-rollouts` know the plugin's location:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
  namespace: argo-rollouts
data:
  trafficRouterPlugins: |-
    - name: "argoproj-labs/openshift"
      location: "file://CHANGE-ME/rollouts-trafficrouter-openshift/openshift-route-plugin"
binaryData: {}
```

5. Create the `CR/Rollout` and put it into the operated services` namespace:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: rollouts-demo
  namespace: rollouts-demo
spec:
    replicas: 2
    selector:
        matchLabels:
        app.kubernetes.io/instance: rollouts-demo
    strategy:
        canary:
        canaryService: canaryService
        stableService: stableService
        steps:
            - setWeight: 30
            - pause:
                duration: 10
        trafficRouting:
            plugins:
            argoproj-labs/openshift:
                routes:
                - rollouts-demo
                namespace: rollouts-demo
    workloadRef:
        apiVersion: apps/v1
        kind: Deployment
        name: canary
```

6. Enjoy It.

## Contributing

Thanks for taking the time to join our community and start contributing!

- Please familiarize yourself with the [Code of Conduct](/CODE_OF_CONDUCT.md) before contributing.
- Check out the [open issues](https://github.com/argoproj-labs/rollouts-plugin-trafficrouter-openshift/issues).
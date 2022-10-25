# flux-clone

Downloads artifact advertised by Flux's source-controller.

## Example

```
~ kubectl get gitrepository --namespace gitops
NAME      URL                              AGE   READY   STATUS
basic     https://github.com/random/repo   30m   True    stored artifact for revision 'main/b7001a544cf5052fefc80c6c1e0b8d6b454010a3'

~ flux-clone --namespace gitops --name basic
2022/10/25 13:02:49 Starting port forwarding to flux-system/source-controller 8080:80...
2022/10/25 13:02:52 Downloading and untarring the source...
2022/10/25 13:02:52 Downloaded "http://localhost:8080/gitrepository/flux-system/basic/latest.tar.gz"
2022/10/25 13:02:52 Untarred in "/tmp/flux-system-basic-latest-4113538677"
2022/10/25 13:02:52 Ended port forwarding

~ cd /tmp/flux-system-basic-latest-4113538677

/tmp/flux-system-basic-latest-4113538677 ls
flux-manifests/  README.md
```

## Usage

- `local-port` - local port for port-forward (default: `8080`)
- `name` - Source name
- `namespace` - Source namespace (default: `flux-system`)
- `revision` - Source revision (default: `latest`)
- `service-name` - source-controller's Service name (default: `source-controller`)
- `service-namespace` - source-controller's Service namespace (default: `flux-system`)
- `service-port` - source-controller's Service port for port-forward
- `source-type` - type of source to use: `gitrepository`, `helmchart` (default: `gitrepository`)

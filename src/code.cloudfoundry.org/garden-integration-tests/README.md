# Garden Integration Tests

**Note**: This repository should be imported as `code.cloudfoundry.org/garden-integration-tests`.

Tests that run against a remote garden server.

## How to run

1. Set `GDN_BIND_IP` and `GDN_BIND_PORT` to the address/port of your running garden server.

```
export GDN_BIND_IP=10.244.0.2
export GDN_BIND_PORT=7777
```

1. Run the tests against the deployed garden.

```
ginkgo -p -nodes=4
```

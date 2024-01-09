# BOSH’s BPM Support within the Garden-runc-release
BOSH’s workload isolation [BPM](https://bosh.io/docs/bpm/bpm/) presents additional complexities to Garden’s workload isolation and should be approached with full knowledge of the caveats involved.

Since BPM isolates workloads, having an additional level of isolation via Garden requires some thought in order to have a successful deployment.  By default, the Garden-runc-release [does not enable BOSH’s BPM](https://github.com/cloudfoundry/garden-runc-release/blob/develop/jobs/garden/spec#L303-L305).

## Garden-run-release’s BPM Properties
### Primary Properties
Garden-runc-release’s BPM support is implemented primarily through two flags `bpm.enabled` and `garden.additional_bpm_volumes`.
#### `bpm.enabled`
The primary flag [`bpm.enabled`](https://github.com/cloudfoundry/garden-runc-release/blob/develop/jobs/garden/spec#L303-L305) within the Garden spec will allow Garden-runc-release to use BPM. When enabling this flag, use a measure of caution: The containers won't survive a restart of the garden job. This is why `garden.destroy_containers_on_start` should be set to avoid leaking container state.
It’s default value is “false”

#### `garden.additional_bpm_volumes` 
A secondary property [`garden.additional_bpm_volumes`](https://github.com/cloudfoundry/garden-runc-release/blob/develop/jobs/garden/spec#L157-L159), requires `bpm.enabled` to be enabled. This property allows an array of shared writable volumes which will be mounted into the BPM container. Submounts from all mount namespaces in a volume are visible in all containers that have the volume mounted in.
Its default value is left blank.


### Secondary Properties
In addition to the above primary properties, there are some secondary properties that have caveats when enabling BPM:
#### `garden.experimental_use_containerd_mode_for_processes`
When enabling [`garden.experimental_use_containerd_mode_for_processes`](https://github.com/cloudfoundry/garden-runc-release/blob/develop/jobs/garden/spec#L231-L233), please ensure you do NOT have `bpm.enabled` enabled. The two properties are incompatible with one another. The purpose of the property is to use Containerd for container process management. Must be used with containerd_mode also set to true. NOTE: cannot be used in combination with bpm or rootless
By default it is disabled(set to false).

#### `garden.destroy_containers_on_start`
The property [`garden.destroy_containers_on_start`](https://github.com/cloudfoundry/garden-runc-release/blob/develop/jobs/garden/spec#L181-L183) is recommended when enabling bpm via `bpm.enabled`. This is recommend so that container state is not leaked and all containers managed by Garden-runc-release, are destroyed, recreated and managed within the confines of BPM’s isolation. 
By default it is disabled(set to false).


## Garden-runc-release’s BPM implementation details
Here are a few key locations where the afformentioned properties get implmented within the release.

https://github.com/cloudfoundry/garden-runc-release/blob/833f96063083602bc20c398ca23780e0ea91e1ae/jobs/garden/monit#L1-L22

https://github.com/cloudfoundry/garden-runc-release/blob/833f96063083602bc20c398ca23780e0ea91e1ae/jobs/garden/templates/bin/garden_start.erb#L85-L87

https://github.com/cloudfoundry/garden-runc-release/blob/833f96063083602bc20c398ca23780e0ea91e1ae/jobs/garden/templates/bin/garden_stop.erb#L7-L10

https://github.com/cloudfoundry/garden-runc-release/blob/833f96063083602bc20c398ca23780e0ea91e1ae/jobs/garden/templates/bin/bpm-pre-start.erb#L12-L16


<!--
Hi there! Thanks for taking time to open an Issue.
Please read through our issue submission checklist to help you fill out as much of the following information as possible.
https://github.com/cloudfoundry/garden-runc-release/blob/develop/docs/issue-submission-guide.md
Please note that we may be unable to invesigate issues if insufficient information is provided.
-->

## Description
<!--
Provide as much detail about the issue as possible.
-->

## Environment

* garden-runc-release version: <!-- (if upgrading, please note both versions; from and to) -->
* Enabled configuration: <!-- containerd, bpm, oci etc -->
* IaaS:
* Stemcell version: <!-- (if upgrading, please note both versions; from and to) -->
* Kernel version: <!-- (`uname -r` from within VM running Garden prints this information) -->

## Steps to reproduce
<!--
Provide steps to reproduce the issue here.
If you do not have a set of steps to reproduce the issue, please explain in detail what you were doing when you encountered the issue.
-->

## Logs
<!--
Provide any output you think may be useful in understanding/explaining the issue.
The garden log files are found in `/var/vcap/sys/log/garden/` on the VM in which the Garden job is running.

Version >= 1.17.2: Run `/var/vcap/packages/dontpanic/bin/dontpanic` (from within the VM running the Garden job).

Version <= 1.17.1: Run ordnance survey (from within the VM running the Garden job) using the following command: `curl bit.ly/garden-ordnance-survey -sSfL | bash`.

Please provide us with the resulting compressed tar (NOTE: GitHub does not support attaching files larger than 10M).
-->

## Docker image (if relevant)
<!--
If there is a particular docker image related to this issue, please provide a
link to this image.
-->

## Cause
<!--
If known, provide the cause of the issue here.
-->

## Resolution
<!--
If known, provide the resolution to the issue here.
-->

# Opening a issue in Garden-Runc-Release, guide and checklist

## Before you start...
- Please search through our previous issues (open and closed).
- Check out our [docs folder](https://github.com/cloudfoundry/garden-runc-release/blob/develop/docs) for descriptions of common Garden gotchas. 
- If you are moving to a new version, or turning on a new feature flag, please refer to all relevant release notes. Many new flags require VM recreation.
- Take a look at our [publicly viewable backlog](https://www.pivotaltracker.com/n/projects/1158420). (If you find that we are already aware of the problem here
  but do not have a Github issue, please still go ahead and open one to help others.)
- Do as much debugging and research as you can. The more information we have, and the less we have to go back-and-forth with you to get more details
  the faster we can get to the heart of the problem.


## On writing a good Title
When filling out the title of this issue, please be as specific as possible. "Garden failed to start" or "Failed Job: garden" from a BOSH failure log is
not helpful. If possible, please ssh onto the failing VM, and look into logs at `/var/vcap/sys/log/garden` to find a more precise error message.


## On writing a good Description

Provide as much detail about the issue as possible covering at a minimum the following points:

- The visible symptoms
- Was the problem seen on a fresh deploy, a re-deploy or an upgrade?
- Was a recreate triggered?
- What was attempted to "fix" or get by the problem? (if any)


## On providing the Environment

Please give as much detail as possible about the environment and the state of the machine where the problem occurred.

The information should include but is not limited to:

- The garden-runc-release version (if upgrading, please note both versions; from and to)
- The IaaS
- Stemcell version (if upgrading, please note both versions; from and to)
- The Kernel version (`uname -r` from within VM running Garden prints this information)


## On writing the Steps to reproduce

Please attempt to reproduce the issue and provide the steps to do so.
If you were unable to reproduce the issue, please take even greater care in the issue description and explain in detail what you were doing when you encountered the issue.


## On collecting and providing Logs

Provide any output you think may be useful in understanding/explaining the issue.
The garden log files are found in `/var/vcap/sys/log/garden/` on the VM in which the Garden job is running.

Garden provides a tool (ordnance-survey) for collecting debug information and we ask issue submitters to run this for every issue submission. There is unlikely to be
any sensitive information in there, but if you are not comfortable attaching the resulting tar to the issue, save it locally and the team will suggest ways to share it securely.

Run ordnance-survey (from the home dir of the VM running the Garden job) using the following command: `curl bit.ly/garden-ordnance-survey -sSfL | bash`.


## On giving the Cause and Resolution

If you know what's up and have already found a way to solve this, or you suspect you may have, please detail what you tried and provide any useful links.
We are also happy to accept PRs.

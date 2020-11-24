# CPU Entitlement

CPU entitlement describes the percentage of host CPU a particular application instance is ‘entitled’ to use. A user will see CPU performance less than 100% when their usage is within their entitlement, and greater than 100% when above their entitlement.

The amount of CPU an application is entitled to (in Cloud Foundry) is set proportionally to how much memory that application has available to it. For example, an application with access to 256MB of memory on a 512MB machine has access to half of the memory on the machine and so is also entitled to half of the CPU of that machine.

## Configuring CPU entitlement

CPU entitlement, like all other CPU-related limits in garden, is set relative to shares. In Cloud Foundry, by default, shares are set equal to application memory, and (therefore) the maximum number of shares is the total memory on the host. For example a 128MB application instance has 128 shares, and a host with 8096MB of memory has a maximum of 8096 shares.

By configuring the property `experimental_cpu_entitlement_per_share_in_percent`, operators are able to change the amount of CPU an application with a certain number of shares is entitled to. A value of 0.3 would mean that each application has access to 0.3% of the total CPU per share; our 256MB application would have access to 76.8% of the total CPU - in this case an operator has overcommitted on the amount of CPU available (assuming a single core machine).

## Choosing an appropriate value

There are three possible states that emerge from how operators configure `experimental_cpu_entitlement_per_share_in_percent`:

1. undercommitted - applications’ CPU entitlements are guaranteed minimums, but some host CPU might not be used
1. optimal - applications’ CPU entitlements are guaranteed minimums
1. overcommitted - applications are not guaranteed to have access to their entitlement

If you don't set a value to this property it will default to an optimal system.

Since by default the total shares available on a host is equal to the amount of memory on the host, an optimal value for `experimental_cpu_entitlement_per_share_in_percent` is 100% divided by the amount of total memory on the host. For example, a host with 1024MB of memory would have an optimal value of `100 / 1024`, which is 0.0977% per share.

Let’s assume our machine actually has 4 cores and not 1 - how might this change the value we choose? Since the machine now has access to 4 times the amount of CPU as its single core counterpart, we can state that each application may now be entitled to 4 times its previous amount; more formally our new value can be found with `400 / 1024` which is roughly 0.390% per share.

Below are a few examples of this value being configured optimally for machines with different total memory and number of CPU cores.

|         | 256M  | 1024M | 8192M |
| ------- |:-----:|:-----:|:-----:|
| 1 core  | 0.39  | 0.098 | 0.012 |
| 4 cores | 1.563 | 0.39  | 0.049 |
| 8 cores | 3.125 | 0.781 | 0.098 |

# CPU Entitlement

In order for operators and application developers to understand whether their applications are using the CPU resource optimally they need a reliable metric that does not depend on the current state of the Diego cell. For example, the metric should not be influenced by the number of containers running on the cell. 

That is why we introduced a new approach to determine the CPU usage of applications based on CPU entitlement. The CPU entitlement is being controlled through the `experimental_cpu_entitlement_per_share_in_percent` bosh property. For example, if `experimental_cpu_entitlement_per_share_in_percent` is set to `0.3` and the application has 500 CPU shares assigned, than the application would be entitled to 500 * 0.3% = 500 * (0.3/100) = 1.5% CPU time, i.e. each second the application would be entitled to 150 nanoseconds CPU time. Note that this entitlement is agnostic to the number of available CPU cores. However, it is up to the operator to factor available cores in when choosing the value of `experimental_cpu_entitlement_per_share_in_percent`, see below.


## How do I map CPU shares to CPU entitlement
When choosing the value of `experimental_cpu_entitlement_per_share_in_percent` it is important to reason about whether and how you want to overcommit the CPU usage. The general formula to determine an optimal value<sup>[1](#optimal-value-footnote)</sup> is

```
experimental_cpu_entitlement_per_share_in_percent = 100 * number_of_CPU_cores/total_amount_of_CPU_shares
```

We recommend setting the `containers.set_cpu_weight` Diego property to `true` in order to simplify the formula above. When that property is set each container would receive one share per megabyte and thus `total_amount_of_CPU_shares` would equal available memory in megabytes:

```
experimental_cpu_entitlement_per_share_in_percent = 100 * number_of_CPU_cores/total_memory_in_megabytes
```

The table below provides several example configurations and what the optimal `experimental_cpu_entitlement_per_share_in_percent` :

|               | 256M  | 1024M | 8192M |
| ------------- |:-----:|:-----:|:-----:|
| 1 CPU core    | 0.39  | 0.098 | 0.012 |
| 4 CPU cores   | 1.563 | 0.39  | 0.049 |
| 8 CPU cores   | 3.125 | 0.781 | 0.098 |


<a name="optimal-value-footnote">[1]</a> Optimal value means that if the whole cell memory is allocated to containers and the containers are using maximum CPU then the host CPU would be completely utilized. Choosing a higher value would overcommit the CPU, a lower value would undercommit it.

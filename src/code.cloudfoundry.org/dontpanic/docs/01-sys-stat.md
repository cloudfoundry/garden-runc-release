---
title: Sysstat
expires_at: never
tags: [ garden-runc-release ]
---

### Sysstat

In the sysstat folder you can find multiple files containing system statistics (CPU, Memory, I/O, ...) over time.

In order to make use of this information, you need to do the following:

```
export LC_ALL=C
for file in $(ls sysstat/sa[0-9]*) ; do sar -A -f "$file"  >> sa.data.txt; done
```

and then use [`ksar`](https://www.linux.com/news/visualize-sar-data-ksar) to turn the result into pdf graphs.

There are 2 types of files in `sysstat`: `sa*` and `sar*`. The `sa`s are binaries updated every 10 mins or so. The `sar`s are text files generated once per day. Therefore you probably want to parse the `sa` files as they will be more current.

Note: `ksar` seems to dislike some lines in that file and will complain. What you can do is keep removing the zero lines until it is happy.

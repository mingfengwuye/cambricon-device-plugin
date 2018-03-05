#!/bin/sh
#
# No tunable parameters, dagmem is autoconfiguring and uses /etc/modules
# for parameters
#
pcie_module="cambricon_dm"
sudo /sbin/rmmod $pcie_module

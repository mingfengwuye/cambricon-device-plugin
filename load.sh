#!/bin/sh
#here=`dirname $0`
module="cambricon_dm"
device="cambricon_dm"
pcie_module="cambricon_dm"

conflict_module="dwc3_pci"

conflict=`lsmod | awk "\\$1==\"$conflict_module\" {print \\$1}"`
if [ $conflict ];then
echo has module conflict 
echo rm confict 
rmmod $conflict_module
fi
echo insmod module 
/sbin/insmod ./$pcie_module.ko $* || exit 1
chmod 777  /dev/${device}Dev0                                                                        

# Change Log

## [0.1.9] - 2019-5-29

### Added

- Add 'all' option to install all modules

### Fixed

- Read interface fail due to script code in ifcfg

## [0.1.8] - 2018-12-1

### Added

- Enable DHCP port for cell

### Changed

- Disable NetworkManager before link bridge to prevent ssh disconnection

- Migrate bridge configure from interface

## [0.1.7] - 2018-11-3

### Added

- Check default route

## [0.1.6] - 2018-10-2

### Changed

- Install nfs-client/semanage for cell

- Ask if continue when installing fail

## [0.1.5] - 2018-8-17

### Added

- Open magic port TCP 25469 for cell guest initiator service

- Install genisoimage for building cloud-init image

## [0.1.4] - 2018-8-7

### Modified

- Change /dev/kvm owner to chosen user

## [0.1.3] - 2018-8-6

### Added

- modify user/group in /etc/libvirt/qemu.conf before start service

## [0.1.2] - 2018-07-25

### Modified

- Output version

- Install EPEL before yum installing



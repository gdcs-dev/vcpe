#!/bin/sh
# setup-nvram-sim.sh
#
# Creates simulated NVRAM flash block devices using loop devices, so that
# nvram.sh can successfully mount /nvram, /nvram2, /data, and /data_bak in
# the container/simulator environment where real flash hardware is absent.
#
# Images are stored under /run/nvram-sim (tmpfs), so they are recreated fresh
# on every boot — appropriate for a simulator.

SIM_DIR="/run/nvram-sim"

# Size in MiB for each partition image
NVRAM_MB=32
NVRAM2_MB=32
DATA_MB=64
DATA_BAK_MB=16

mkdir -p "$SIM_DIR"

setup_flash() {
    local devname="$1"
    local size_mb="$2"
    local image="${SIM_DIR}/${devname}.img"
    local devpath="/dev/${devname}"

    # Idempotent: skip if the symlink already points to an active loop device
    if [ -L "$devpath" ]; then
        local current
        current=$(readlink -f "$devpath" 2>/dev/null)
        if losetup "$current" >/dev/null 2>&1; then
            echo "setup-nvram-sim: $devpath already set up, skipping"
            return 0
        fi
        rm -f "$devpath"
    fi

    # Create a sparse image file (allocates no real disk space until written)
    dd if=/dev/zero of="$image" bs=1M count=0 seek="$size_mb" 2>/dev/null
    if [ $? -ne 0 ]; then
        echo "setup-nvram-sim: ERROR: could not create image $image"
        return 1
    fi

    # Format as ext4
    mkfs.ext4 -F -L "$devname" "$image" >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "setup-nvram-sim: ERROR: mkfs.ext4 failed for $image"
        return 1
    fi

    # Find the next free loop device number and ensure its /dev node exists,
    # then attach the image to that specific device.  Using --find and then
    # losetup <dev> <image> as two steps (rather than --find --show) lets us
    # create the missing device node in between.
    local loopdev
    loopdev=$(losetup --find 2>/dev/null)
    if [ -z "$loopdev" ]; then
        echo "setup-nvram-sim: ERROR: no free loop device available"
        return 1
    fi
    local loopnum="${loopdev#/dev/loop}"
    [ -b "$loopdev" ] || mknod "$loopdev" b 7 "$loopnum" 2>/dev/null
    if ! losetup "$loopdev" "$image" 2>/dev/null; then
        echo "setup-nvram-sim: ERROR: losetup failed for $image on $loopdev"
        return 1
    fi

    # Create symlink so nvram.sh finds /dev/flash-rdknvramo etc.
    ln -sf "$loopdev" "$devpath"
    echo "setup-nvram-sim: $devpath -> $loopdev (${size_mb}MiB ext4)"
}

setup_flash "flash-rdknvramo" "$NVRAM_MB"
setup_flash "flash-rdknvramb" "$NVRAM2_MB"
setup_flash "flash-rgnonvolo" "$DATA_MB"
setup_flash "flash-rgnonvolb" "$DATA_BAK_MB"

# factory_data_write.sh loops 100×2 s waiting for /nvram/secure/oemdata.
# Pre-create it inside the image before nvram.sh mounts it at /nvram.
tmp_mnt=$(mktemp -d "${SIM_DIR}/mnt-XXXXXX")
if mount /dev/flash-rdknvramo "$tmp_mnt" 2>/dev/null; then
    mkdir -p "${tmp_mnt}/secure/oemdata"
    umount "$tmp_mnt"
fi
rmdir "$tmp_mnt" 2>/dev/null

exit 0

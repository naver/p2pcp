#!/bin/bash
#
# Create random test data with various file sizes and permissions
#
# Usage: create_random_data.sh <target_dir>
#

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <target_dir>" >&2
    exit 1
fi

TARGET_DIR="$1"

mkdir -p "$TARGET_DIR/big" || { echo "Error: Failed to create big directory" >&2; exit 1; }
mkdir -p "$TARGET_DIR/tiny" || { echo "Error: Failed to create tiny directory" >&2; exit 1; }

# Create large sparse files with random data at the beginning
for i in {1..4}; do
    file="$TARGET_DIR/big/64m_rand_file_$(printf "%03d" "$i")"
    truncate -s 64M "$file" || { echo "Error: Failed to create $file" >&2; exit 1; }
    head -c 512K < /dev/urandom | dd of="$file" conv=notrunc status=none || { echo "Error: Failed to write random data to $file" >&2; exit 1; }
done

# Create small random files
for i in {1..256}; do
    file="$TARGET_DIR/tiny/16k_rand_file_$(printf "%04d" "$i")"
    head -c 16K < /dev/urandom > "$file" || { echo "Error: Failed to create $file" >&2; exit 1; }
done

# Set various permissions to test permission preservation
chmod 600 "$TARGET_DIR/tiny/16k_rand_file_0100"
chmod 601 "$TARGET_DIR/tiny/16k_rand_file_0101"
chmod 602 "$TARGET_DIR/tiny/16k_rand_file_0102"
chmod 603 "$TARGET_DIR/tiny/16k_rand_file_0103"
chmod 604 "$TARGET_DIR/tiny/16k_rand_file_0104"
chmod 605 "$TARGET_DIR/tiny/16k_rand_file_0105"
chmod 606 "$TARGET_DIR/tiny/16k_rand_file_0106"
chmod 607 "$TARGET_DIR/tiny/16k_rand_file_0107"
chmod 610 "$TARGET_DIR/tiny/16k_rand_file_0108"
chmod 620 "$TARGET_DIR/tiny/16k_rand_file_0109"
chmod 630 "$TARGET_DIR/tiny/16k_rand_file_0110"
chmod 640 "$TARGET_DIR/tiny/16k_rand_file_0111"
chmod 650 "$TARGET_DIR/tiny/16k_rand_file_0112"
chmod 660 "$TARGET_DIR/tiny/16k_rand_file_0113"
chmod 670 "$TARGET_DIR/tiny/16k_rand_file_0114"
chmod 400 "$TARGET_DIR/tiny/16k_rand_file_0115"
chmod 500 "$TARGET_DIR/tiny/16k_rand_file_0116"
chmod 600 "$TARGET_DIR/tiny/16k_rand_file_0117"
chmod 700 "$TARGET_DIR/tiny/16k_rand_file_0118"

# Create edge cases
mkdir -p "$TARGET_DIR/empty" || { echo "Error: Failed to create empty directory" >&2; exit 1; }
touch "$TARGET_DIR/zero_length" || { echo "Error: Failed to create zero_length file" >&2; exit 1; }
ln -sf ../big/64m_rand_file_001 "$TARGET_DIR/tiny/sym_link" || { echo "Error: Failed to create symlink" >&2; exit 1; }

echo "Test data created successfully"

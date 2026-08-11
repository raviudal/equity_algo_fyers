#!/bin/bash
set -e

echo "=========================================================="
echo " Setting up GCP e2-micro VM for AlgoEngine Trading Core"
echo "=========================================================="

# 1. Create 1GB Linux Swap File to safeguard memory on e2-micro
if [ ! -f /swapfile ]; then
    echo "[1/4] Creating 1 GB Swap memory file..."
    sudo fallocate -l 1G /swapfile || sudo dd if=/dev/zero of=/swapfile bs=1M count=1024
    sudo chmod 600 /swapfile
    sudo mkswap /swapfile
    sudo swapon /swapfile
    echo '/swapfile swap swap defaults 0 0' | sudo tee -a /etc/fstab
    echo "[+] 1 GB Swap active!"
else
    echo "[1/4] Swap file already exists."
fi

# 2. Create Application Directory
echo "[2/4] Setting up /opt/algoengine directory..."
sudo mkdir -p /opt/algoengine
sudo chown -R $USER:$USER /opt/algoengine

# 3. Copy service unit file to systemd
echo "[3/4] Installing systemd service..."
if [ -f algoengine.service ]; then
    sudo cp algoengine.service /etc/systemd/system/algoengine.service
    sudo systemctl daemon-reload
    sudo systemctl enable algoengine
fi

# 4. Open Firewall Port 8080 (ufw if active)
if command -v ufw > /dev/null; then
    sudo ufw allow 8080/tcp || true
fi

echo "=========================================================="
echo " GCP VM Setup Complete!"
echo " Copy compiled 'algoengine' binary and '.env' to /opt/algoengine/"
echo " Then run: sudo systemctl start algoengine"
echo " Access Dashboard: http://<YOUR_VM_EXTERNAL_IP>:8080"
echo "=========================================================="

Vagrant.configure(2) do |config|
	config.ssh.forward_agent = true
	config.ssh.username = 'vagrant'
	config.vm.network "private_network", ip: "192.168.219.2"

	config.vm.synced_folder ".", "/vagrant", disabled: true

	config.vm.define "mgmt-dev" do |instance|
		instance.vm.box = "bento/fedora-31"
	end

	config.vm.provider "virtualbox" do |v|
		v.memory = 1536
		v.cpus = 2
	end
	config.vm.provider "libvirt" do |v|
		v.memory = 2048
	end

	config.vm.provision "file", source: "vagrant/motd", destination: ".motd"
	config.vm.provision "shell", inline: "cp ~vagrant/.motd /etc/motd"

	config.vm.provision "file", source: "vagrant/mgmt.bashrc", destination: ".mgmt.bashrc"
	config.vm.provision "file", source: "~/.gitconfig", destination: ".gitconfig"

	config.vm.provision "shell", inline: "dnf install -y golang git make"

	# set up packagekit
	config.vm.provision "shell" do |shell|
		shell.inline = <<-SCRIPT
			dnf install -y PackageKit
			systemctl enable packagekit
			systemctl start packagekit
		SCRIPT
	end

	# set up vagrant home
	script = <<-SCRIPT
		grep -q 'mgmt\.bashrc' ~/.bashrc || echo '. ~/.mgmt.bashrc' >>~/.bashrc
		. ~/.mgmt.bashrc
		mkdir -p ~/gopath/src/github.com/purpleidea
		cd ~/gopath/src/github.com/purpleidea
		git clone https://github.com/purpleidea/mgmt --recursive
		cd mgmt
		make deps
	SCRIPT
	config.vm.provision "shell" do |shell|
		shell.privileged = false
		shell.inline = script
	end
end

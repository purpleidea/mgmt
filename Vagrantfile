Vagrant.configure(2) do |config|
	config.ssh.forward_agent = true
	config.ssh.username = 'vagrant'
	config.vm.network "private_network", ip: "192.168.219.2"

	config.vm.synced_folder ".", "/vagrant", disabled: true

	config.vm.define "mgmt-dev" do |instance|
		instance.vm.box = "fedora/24-cloud-base"
	end

	config.vm.provider "virtualbox" do |v|
		v.memory = 1536
		v.cpus = 2
	end

	config.vm.provision "file", source: "vagrant/motd", destination: ".motd"
	config.vm.provision "shell", inline: "cp ~vagrant/.motd /etc/motd"

	config.vm.provision "file", source: "vagrant/mgmt.bashrc", destination: ".mgmt.bashrc"
	config.vm.provision "file", source: "~/.gitconfig", destination: ".gitconfig"

	# copied from make-deps.sh (with added git)
	config.vm.provision "shell", inline: "dnf install -y libvirt-devel golang golang-googlecode-tools-stringer hg git"

	# set up vagrant home
	script = <<-SCRIPT
		grep -q 'mgmt\.bashrc' ~/.bashrc || echo '. ~/.mgmt.bashrc' >>~/.bashrc
		. ~/.mgmt.bashrc
		go get -u github.com/purpleidea/mgmt
		cd ~/gopath/src/github.com/purpleidea/mgmt
		make deps
	SCRIPT
	config.vm.provision "shell" do |shell|
		shell.privileged = false
		shell.inline = script
	end
end

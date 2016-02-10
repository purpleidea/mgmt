#!/usr/bin/python

# modified from:
# https://github.com/kbsingh/centos-ci-scripts/blob/master/build_python_script.py
# usage: centos-ci.py giturl [branch [commands]]

import os
import sys
import json
import urllib
import subprocess

# static argv to be used if running script inline
argv = [
	#'https://github.com/purpleidea/mgmt', # giturl
	#'master',
	#'make test',
]
argv.insert(0, '') # add a fake argv[0]
url_base = 'http://admin.ci.centos.org:8080'
apikey = '' # put api key here if running inline
if apikey == '':
	apikey = os.environ.get('DUFFY_API_KEY')
if apikey is None or apikey == '':
	apikey = open('duffy.key', 'r').read().strip()
ver = '7'
arch = 'x86_64'
count = 1

if len(argv) <= 1: argv = sys.argv # use system argv because ours is empty
if len(argv) <= 1:
	print 'Not enough arguments supplied!'
	sys.exit(1)

git_url = argv[1]
branch = 'master'
if len(argv) > 2: branch = argv[2]
folder = os.path.basename(git_url)	# should be project name
run = 'make vtest' # the omv vtest cmd is a good option to run from this target
if len(argv) > 3: run = ' '.join(argv[3:])

get_nodes_url = "%s/Node/get?key=%s&ver=%s&arch=%s&i_count=%s" % (url_base, apikey, ver, arch, count)
data = json.loads(urllib.urlopen(get_nodes_url).read()) # request host(s)
hosts = data['hosts']
ssid = data['ssid']
done_nodes_url = "%s/Node/done?key=%s&ssid=%s" % (url_base, apikey, ssid)

host = hosts[0]
ssh = "ssh -tt -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o SendEnv=JENKINS_URL root@%s" % host
yum = 'yum -y install git wget'
omv = 'wget https://github.com/purpleidea/oh-my-vagrant/raw/master/extras/install-omv.sh && chmod u+x install-omv.sh && ./install-omv.sh && wget https://github.com/purpleidea/mgmt/raw/master/misc/make-gopath.sh && chmod u+x make-gopath.sh && ./make-gopath.sh'
cmd = "%s '%s && %s'" % (ssh, yum, omv) # setup
print cmd
r = subprocess.call(cmd, shell=True)
if r != 0:
	# NOTE: we don't clean up the host here, so that it can be inspected!
	print "Error configuring omv on: %s" % host
	sys.exit(r)

# the second ssh call will run with the omv /etc/profile.d/ script loaded
git = "git clone --recursive %s %s && cd %s && git checkout %s" % (git_url, folder, folder, branch)
cmd = "%s 'export JENKINS_URL=%s && %s && %s'" % (ssh, os.getenv('JENKINS_URL', ''), git, run) # run
print cmd
r = subprocess.call(cmd, shell=True)
if r != 0:
	print "Error running job on: %s" % host

output = urllib.urlopen(done_nodes_url).read() # free host(s)
if output != 'Done':
	print "Error freeing host: %s" % host

sys.exit(r)

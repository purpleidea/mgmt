#!/bin/sh

set -e

user() {
	cmd="$1"
	shift

	case "$cmd" in
		check)
			user="$1"
			echo "\\du ${user}" | psql --csv | grep -q "^${user},"
			;;
		create)
			user="$1"
			echo "> Creating user in postgres: ${user}"
			echo "CREATE USER ${user};" | psql
			;;
		password)
			user="$1"
			password="$2"
			echo "> Setting user password in postgres: ${user}"
			echo "ALTER USER ${user} WITH PASSWORD '${password}';" | psql
			;;
		*)
			echo "$0> Unknown user command: ${cmd}"
			exit 1
			;;
	esac
}

db() {
	cmd="$1"
	shift

	case "$cmd" in
		check)
			db="$1"
			echo "\\l ${db}" | psql --csv | grep -q "^${db},"
			;;
		create)
			db="$1"
			user="$2"
			echo "> Creating postgres database: ${db}"
			echo "CREATE DATABASE moodle WITH OWNER '${user}'" | psql
			;;
		set-owner)
			db="$1"
			owner="$2"
			echo "> Setting owner of db ${db} to ${owner}"
			echo "ALTER DATABASE ${db} OWNER TO ${owner}" | psql
			;;
		*)
			echo "$0> Unknown db command: ${cmd}"
			exit 1
			;;
	esac
}

init() {
	# mgmt has many operations going on in parallel, and the postgresql-setup
	# will abort if systemd claims the daemon needs to be reloaded.
	# So try a short loop reloading it if needed before we begin.
	while [ "$(systemctl show -p NeedDaemonReload postgresql.service)" != "NeedDaemonReload=no" ] ; do
		echo "db init> running systemctl daemon-reload"
		systemctl daemon-reload
		sleep 1;
	done

	postgresql-setup --initdb --port 5432 --unit postgresql
}

if [ -z "$1" ] ; then
	echo "Usage: $0 <cmd>"
	echo "  user create <user>"
	echo "  user check <user>"
	echo "  db check <db>"
	echo "  db create <db> <user>"
	exit 1
fi

cmd="$1"
shift

case "$cmd" in
	user) user "$@" ;;
	db) db "$@" ;;
	init) init "$@" ;;
	*) echo "$0: Unknown command: $cmd "; exit 1 ;;
esac

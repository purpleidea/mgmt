/* NOTES:
 *
 * https://copr.fedorainfracloud.org/coprs/g/cockpit/cockpit-preview/
 * sudo dnf copr enable @cockpit/cockpit-preview
 * sudo dnf install cockpit
 * npm install <whatever>
 *
 * cockpit-bridge --packages	// list what's there
 * ~/.local/share/cockpit/mgmt/	// contains manifest.json mgmt.html
 *
 * ./node_modules/.bin/webpack
 * ./node_modules/.bin/webpack --watch	// make it live
 *
 * // will look in ~/.local/share/cockpit/ for stuff
 * sudo systemctl start cockpit.service
 *
 * http://localhost:9090/mgmt/mgmt
 * http://localhost:9090/cockpit/@localhost/mgmt/mgmt.html#/
 *
 * ./mgmt run --yaml cockpit/mgmt.yaml --tmp-prefix	// run it :)
 * watch -n 1 cat mgmt.yaml
 * watch -n 1 'virsh list --all'
 *
 */

console.log("mgmt + COCKPIT");

require("./patterns.js"); // additional cockpit provided widgets

var jsyaml = require('js-yaml'); // r.js is needed to require

var YAML = { stringify: jsyaml.safeDump, parse: jsyaml.safeLoad }; // thanks mvollmer

var basepath = "/home/james/.local/share/cockpit/mgmt/"; // XXX: is there an API to get this path?

var N = 5; // between 0-5 machines

var isLive = false;

$(document).ready(main); // required!

function main() {

	$("#save").on('click', write);

	file = cockpit.file(basepath + "mgmt.yaml", { syntax: YAML });

	file.watch(watch); // mvollmer says: we are guaranteed to get one event here on load

	promise = file.read(); // XXX: read once anyways...
	promise
	.done(write)
	.fail(error);

	$("#live").on('change', updatelive);
	$("#myslider").on('change', updateslider);
	$("#myslidertext").on('change', updateslidertext);

}

function updatelive(event) {
	isLive = event.currentTarget.checked; // global
	console.log("isLive: " + isLive);
}

function updateslider(event) {
	$("#myslidertext").val( event.currentTarget.value );
	$("#mysliderint").val( Math.ceil(event.currentTarget.value * N) );
	if (isLive) {
		write(); // run the write
	}
}

function updateslidertext(event) {
	console.log("XXX: ", parseFloat(event.currentTarget.value));
	$("#myslider").prop("value", parseFloat(event.currentTarget.value)); // XXX: doesn't work
}

function watch(content, tag, error) {
	// XXX: if error ...
	console.log("FILE CHANGED", content, tag);
	read(content, tag)
}

function read(content, tag) {
	console.log("Data:", content.hello);
	$("#comment").val( content.comment );
	$("#myslidertext").val( parseFloat(content.ratio) );
}

function virt(name, state) {
	return {
		"name": name,
		"uri": "qemu:///session",
		"cpus": 1,
		"memory": 524288, // 512 mb
		"state": state, // running, paused, shutoff
		"transient": true,
	}
}

function yamlfile(virtN) {

	var virtArray = [];
	for (i = 0; i < N; i++) {
		var state = (i < virtN) ? "running" : "shutoff";
		virtArray.push(virt("mgmt" + i, state ));
	}
	return {
		"graph": "name",
		"resources": {
			"noop": [
			],
			"virt": virtArray,
		},
		"edges": [],
	}
}

function write() {
	console.log("Writing...");
	var ratio = parseFloat($("#myslidertext").val());
	var number = Math.ceil(ratio * N);
	newcontent = yamlfile(number);
//	newcontent = {
//		"hello": "world",
//		"ratio": parseFloat($("#myslidertext").val()),
//		"comment": $("#comment").val(),
//	};
	newcontent["hello"] = "world";
	newcontent["ratio"] = ratio;
	newcontent["number"] = number;
	newcontent["comment"] = $("#comment").val();

	promise = file.replace(newcontent); // [ expected_tag ])
	promise
	.done(function (new_tag) {
		$("#info").val( "file updated successfully!" );
	})
	.fail(error);
}

function error(err) {
	$("#info").val( "file error: " + err );
}

//promise = file.replace(content, [ expected_tag ])
//promise
//    .done(function (new_tag) { ... })
//    .fail(function (error) { ... })

//promise = file.modify(callback, [ initial_content, initial_tag ]
//promise
//    .done(function (new_content, new_tag) { ... })
//    .fail(function (error) { ... })

//file.close()

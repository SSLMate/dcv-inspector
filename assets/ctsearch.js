// Copyright (C) 2024 Opsmate, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
//
// Except as contained in this notice, the name(s) of the above copyright
// holders shall not be used in advertising or otherwise to promote the
// sale, use or other dealings in this Software without prior written
// authorization.

async function ctsearch() {
	function make_message_row(message) {
		let tr = document.createElement("tr");
		let td = document.createElement("td");
		td.colSpan = 5;
		td.style.textAlign = "center";
		td.innerText = message;
		tr.appendChild(td);
		return tr;
	}
	function make_text_cell(message) {
		let td = document.createElement("td");
		td.innerText = message;
		return td;
	}
	function make_list_cell(items) {
		let td = document.createElement("td");
		let ul = document.createElement("ul");
		for (const item of items) {
			let li = document.createElement("li");
			li.innerText = item;
			ul.appendChild(li);
		}
		td.appendChild(ul);
		return td;
	}
	function make_link(label, href) {
		let a = document.createElement("a");
		a.href = href;
		a.innerText = label;
		return a;
	}
	function make_link_cell(issuance) {
		let td = document.createElement("td");
		td.appendChild(make_link("PEM", "https://api.certspotter.com/v1/issuances/"+issuance.id+".pem"));
		td.appendChild(document.createTextNode(" "));
		td.appendChild(make_link("View", "/view_issuance?id="+issuance.id));
		td.appendChild(document.createTextNode(" "));
		td.appendChild(make_link("crt.sh", "https://crt.sh/?sha256="+issuance.cert_sha256));
		return td;
	}

	const testID = document.body.dataset.testId;
	const tbody = document.getElementById("ctsearch_results");
	tbody.innerHTML = "";
	tbody.appendChild(make_message_row("Searching..."));
	let once = false;
	let after = "";
	while (true) {
		let response;
		try {
			response = await fetch("/test/"+testID+"?ctsearch=1&ctsearch_after="+after);
		} catch (error) {
			if (!once) { tbody.innerHTML = ""; }
			tbody.appendChild(make_message_row(error.message));
			return;
		}
		if (!response.ok) {
			if (!once) { tbody.innerHTML = ""; }
			tbody.appendChild(make_message_row(await response.text()));
			return;
		}
		const issuances = await response.json();
		if (!once) { tbody.innerHTML = ""; }
		if (issuances.length == 0) {
			if (!once) {
				tbody.appendChild(make_message_row("No certificates found"));
			}
			return;
		}
		once = true;
		for (const issuance of issuances) {
			let row = document.createElement("tr");
			row.appendChild(make_text_cell(issuance.id));
			row.appendChild(make_text_cell(issuance.issuer.operator ? issuance.issuer.operator.name : "Unknown"));
			row.appendChild(make_list_cell(issuance.dns_names));
			row.appendChild(make_text_cell(issuance.revoked === true ? "Revoked" : "Valid"));
			row.appendChild(make_link_cell(issuance));
			tbody.appendChild(row);
			after = issuance.id;
		}
	}
}

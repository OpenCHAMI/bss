<!-- SPDX-FileCopyrightText: Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Boot credential logging

Boot parameters, cloud-init data, referral/join tokens and signed image URLs can
contain credentials. This patch removes payloads from the selected boot data
storage and rendering diagnostics at normal and debug levels. Operation events
and fixed failure classifications replace the payload-bearing messages in these
paths. It does not audit every service or dependency log.

This change preserves HTTP routes, response bodies/status codes, discovery,
MAC/NID/name lookup precedence, storage and signer control flow. In particular,
GET `/boot/v1/dumpstate`, `/boot/v1/endpoint-history` and `/boot/v1/hosts` remain
anonymous when optional bootparameters authentication is configured. No CLI,
configuration, credential or dependency migration is required.

This is a diagnostic-output fix, not complete API authorization or error-response
redaction. Existing error responses and public data remain unchanged, including
raw storage/signer error details; those require separate contract review. Logs
retained from older versions may already contain sensitive values. Log parsers
that depended on complete boot payloads or raw failure text must use the remaining
operation/failure events instead.

<!-- SPDX-FileCopyrightText: Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC -->
<!-- SPDX-License-Identifier: MIT -->

# Boot credential logging

Boot parameters, cloud-init data, referral/join tokens and signed image URLs can
contain credentials. Boot parameter request diagnostics, memory/SQL storage
diagnostics, boot-script rendering and cloud-data merge failures omit those
payloads at normal and debug log levels. S3 configuration, parameter replacement,
phone-home storage and boot-script signer failures use fixed operation/failure
events rather than raw credential-bearing errors. Dumpstate response encoding or
write failures log `event=dumpstate_response_encode_failed` with the fixed
`error_class=response_encode_error`, omitting the response inventory and raw error.

GET `/boot/v1/dumpstate`, `/boot/v1/endpoint-history` and `/boot/v1/hosts` are
anonymous, including when optional bootparameters authentication is configured.
Dumpstate returns stored boot parameters and image URLs, which can contain
credentials. Anonymous access is existing behavior, not a guarantee that the
returned data is safe to disclose outside the deployment's trusted network.

HTTP response bodies/status codes, discovery, MAC/NID/name lookup precedence,
storage and signer behavior are unchanged. Some storage/signer errors are still
returned verbatim in HTTP error responses and may expose sensitive values.
Other service/dependency logs are outside the guarantees described here. Retained
logs from older versions may contain credentials; changing diagnostic output does
not remove old logs or rotate exposed credentials. Log consumers must use the
remaining operation/failure events rather than full boot payloads or raw failures.

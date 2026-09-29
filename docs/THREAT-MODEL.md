# Threat model (v0.1)

## Assets

- GPU vendor API keys
- S3 credentials / data (projects may include unpublished creative work)
- Session endpoints (ComfyUI without auth is dangerous on public IP)

## Threats & mitigations

| Threat | Mitigation |
|--------|------------|
| Stolen provider key from phone | Target: keys only in the control-plane vault; clients use a user session JWT. P1/P3: keys stay in the operator's `.env` or process environment for the CLI. The PWA and Tauri shell do not accept or persist them. |
| Client webview stores provider or S3 keys | The UI has no secret fields. localStorage holds project ids, the `wckd` path, a config path, and a working directory. GPU actions in the browser wait on `HttpBridge`. |
| Orphan GPU burn | Hard deadline + independent sweeper + push alert |
| Public ComfyUI takeover | Tunnel + optional basic auth / token; no raw bind to 0.0.0.0 by default |
| Partial S3 upload on kill | Drain gate before terminate; quarantine hold on failure |
| Cross-tenant data leak | Per-project prefixes; no shared volumes |
| Supply-chain malicious preset image | Pin digests; allow only signed/org images in prod |
| Insider control-plane | Encrypt secrets at rest; audit log; least privilege IAM |

## Out of scope for v0.1

- Formal pen-test
- SOC2
- Multi-tenant hard isolation proof

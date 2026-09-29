# Threat model (v0.1)

## Assets

- GPU vendor API keys
- S3 credentials / data (projects may include unpublished creative work)
- Session endpoints (ComfyUI without auth is dangerous on public IP)

## Threats & mitigations

| Threat | Mitigation |
|--------|------------|
| Stolen provider key from phone | Keys only in control-plane vault; clients use user session JWT |
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

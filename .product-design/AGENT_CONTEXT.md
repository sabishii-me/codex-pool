# Product design review context

This directory is project-owned product design state managed with Product Design Harness.

Before changing a direction or artifact, read:

1. `direction/product-truth.md`;
2. `direction/frozen-design-prompt.md`;
3. `direction/artifact-contract.md`;
4. `direction/state-matrix.md`;
5. `direction/fixtures.json`;
6. the target round and artifact manifests;
7. the target artifact README.

Rules:

- Do not create multiple alternatives before explicit Step 0 acceptance.
- Do not mark a direction passed or frozen without owner approval.
- Designer artifacts contain simulated product UX only.
- Inspection, feedback, ratings, viewport/runtime controls, persistence, screenshots, and candidate comparison belong to the shared harness.
- Keep prior rounds and owner feedback intact.
- Runtime profiles identify presentation context; they do not grant authority.
- Feedback remains product state until the owner explicitly authorizes implementation or submission to an agent.
- Never silently substitute a blocked artifact or designer.
- Run `product-design-harness doctor` and `product-design-harness validate`; run browser validation/capture when available before claiming review readiness.

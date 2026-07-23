# Step 0 required state matrix

Status: draft for owner review

| ID | Viewport · runtime | Starting context | Transition or inspection | Acceptance evidence |
|---|---|---|---|---|
| S1 | Desktop · member | Dashboard · healthy | Review overview, change activity range, use quick actions | Usability, personal scope, availability, activity, provider summary, and next actions are clear within ten seconds |
| S2 | Desktop · operator | Dashboard · degraded | Enter Monitor and inspect the connection incident | Operations are visibly separate; Monitor explains impact, routing compensation, and recovery without placing configuration on the dashboard |
| S3 | Desktop · member | Models | Search and filter fictional routes, simulate copy | Route ID, provider, capability, context, and availability remain understandable; operator connection details are absent |
| S4 | Tablet portrait · operator | Dashboard · stale analytics | Review localized stale state and open navigation | Last-known-good information remains visible, freshness is explicit, and operator destinations do not crowd member priorities |
| S5 | Mobile portrait · member | Dashboard · healthy | Scroll overview and use bottom navigation | Status and primary action appear early; no root horizontal overflow; secondary detail is progressively disclosed |
| S6 | Mobile portrait · operator | Dashboard · degraded | Open More, enter Monitor, inspect incident | Member navigation remains compact; operator tools are reachable but not mixed into the primary bottom bar |
| S7 | Mobile landscape · member | Models | Search/filter with focused input and open route details | Input and actions remain visible, content scrolls in a named region, and no root horizontal overflow occurs |
| S8 | Desktop · operator | Monitor · localized resource error | Retry simulated persistence panel | Healthy monitor panels remain visible; the failing resource has an actionable local error and simulated recovery |
| S9 | Any · member | Dashboard · no actionable incidents | Inspect attention region | A concise “No action required” state replaces an empty alert list without inventing warnings |
| S10 | Any · operator | Provider connection summary · unknown provider | Inspect provider presentation | Unknown provider identity remains readable with neutral presentation and no provider-specific component failure |

## Shared acceptance checks

- One meaningful `h1` per page and logical heading order.
- Header, navigation, main content, and contextual regions use semantic landmarks.
- Every interactive control is keyboard reachable and visibly focused.
- Current navigation and pressed filter/range state are programmatically exposed.
- Status uses text and icon/shape in addition to color.
- Unknown, zero, stale, unavailable, measured, and inferred are distinguishable.
- Fictional scope is visible; no state implies a live operation.
- Runtime query values alter simulated visibility only and are never described as authorization.
- Reload restores deterministic fixture state.
- Reduced-motion preference removes nonessential animation.

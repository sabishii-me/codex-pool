# Step 0 required state matrix

Status: draft for owner review

| ID | Viewport · runtime | Starting context | Transition or inspection | Acceptance evidence |
|---|---|---|---|---|
| S1 | Desktop · member | Dashboard · healthy | Review overview, change activity range, use quick actions | Usability, personal scope, availability, activity, provider summary, and next actions are clear within ten seconds |
| S2 | Desktop · operator | Dashboard · degraded | Enter Monitor and inspect the connection incident | Operations are visibly separate; Monitor explains impact, routing compensation, and recovery without placing configuration on the dashboard |
| S3 | Desktop · member | Models and My usage | Search/filter fictional routes, simulate copy, inspect weekly leaderboard | Route details remain understandable; usage comparison is aggregate, abbreviated, and clearly distinct from personal totals |
| S4 | Tablet portrait · operator | Dashboard · stale analytics | Review localized stale state, collapse/expand navigation, and open destinations | Last-known-good information remains visible, freshness is explicit, and navigation remains usable in either density |
| S5 | Mobile portrait · member | Dashboard · healthy | Review gateway status and use bottom navigation | Personal status and usage appear early; no root horizontal overflow; secondary detail is progressively disclosed |
| S6 | Mobile portrait · operator | Monitor · degraded | Inspect the live graph and incident, then open More | Monitor is the default compact destination; live state precedes administration and operator tools remain reachable |
| S7 | Mobile landscape · member/operator | Personal usage or Monitor focus | Observe the dominant graph and open compact navigation | The compact shell defaults to the role-relevant live view, actions remain visible, and no root horizontal overflow occurs |
| S8 | Tablet landscape · member/operator | Secondary-screen focus view | Leave the display running and inspect current state at a glance | One large graph dominates; freshness and four role-scoped metrics remain readable without interaction; broad dashboard panels are suppressed |
| S9 | Desktop · operator | Monitor · localized resource error | Retry simulated persistence panel | Healthy monitor panels remain visible; the failing resource has an actionable local error and simulated recovery |
| S10 | Any · member | Dashboard · no actionable incidents | Inspect attention region | A concise “No action required” state replaces an empty alert list without inventing warnings |
| S11 | Any · operator | Provider connection summary · unknown provider | Inspect provider presentation | Unknown provider identity remains readable with neutral presentation and no provider-specific component failure |

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

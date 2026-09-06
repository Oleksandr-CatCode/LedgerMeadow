# Screenshot provenance

These JPEGs are browser captures of the existing LedgerMeadow React components. They show an isolated local renderer supplied with newly authored, in-memory example responses. They are visual demonstrations, not results from a live banking connection or financial-engine calculation.

The renderer was kept outside the application repository. It did not load local environment files, call Clerk, use a database, or contact a bank provider. API reads were answered from invented fixtures; writes were rejected. A content security policy blocked network connections and embedded frames. No authentication bypass or screenshot mode was added to the shipped application.

Every image contains a visible **SYNTHETIC DEMO** banner. The identity is “Demo workspace”; the institution is “Example Credit Union”. Account suffixes are dummy sequence values. Names, dates in 2040, amounts, balances, and allocations were authored for the demonstration and have no connection to a person's financial records.

| File | View |
| --- | --- |
| `dashboard.jpg` | Available balance, protected money, spaces, monthly plan, and upcoming items |
| `accounts.jpg` | An invented institution, spending account, and savings account |
| `spaces.jpg` | Invented everyday, bills, and rainy-day allocations |
| `planning.jpg` | Invented expected income, monthly allocations, recurring costs, and upcoming commitments |

The images were reviewed visually and their visible text was compared with the invented fixture inputs. Their JPEG metadata contains only a generic JFIF header and a shared standard color profile; no EXIF, location, user comments, embedded thumbnail, private URL, or appended payload was found.

`scripts/check_publication.py` pins the exact SHA-256 digest and path of each approved image. Other images remain blocked, and changing a screenshot's pixels or metadata causes the check to fail. Hash verification preserves a completed review; it cannot determine whether a new screenshot is private.

For future captures, render only newly authored synthetic data in an isolated browser session. Review the complete visible image and metadata, run the privacy scans, and update an image digest only after that review. Never capture a working financial session and rely on cropping, masking, or blurring private fields.

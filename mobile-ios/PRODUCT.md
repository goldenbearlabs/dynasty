<!-- Records the confirmed product purpose and constraints for the native iPhone client. -->
<!-- impeccable:product-schema 1 -->

# Mosaic Fantasy for iPhone

## Platform

ios

## Users

Fantasy-sports managers who select a league, follow standings and activity, and later manage teams and league play. Commissioners use the same account system; their administrative screens are a later mobile chunk.

## Product Purpose

Bring the existing Mosaic Fantasy league experience to a native iPhone app, one functional section at a time, using the same Go API and league data as the website.

## Operating Context

Managers may belong to no playable league, may be awaiting email verification, or may have a commissioner invite. A playable league must match the current gameplay dynasty before its data can be shown. The app is used during live sports and between events, including in dark environments.

## Capabilities and Constraints

- The first milestone covers account entry, Leagues selection, and League standings and activity.
- iOS 17 or later; iPhone layout first. iPad optimization is deferred.
- Keep mobile bearer tokens in Keychain and never include the server's admin token.
- Use the production HTTPS API for Release and the local API for Debug.
- The website, Go API, and database remain shared and unchanged in this milestone.
- Preserve the unfinished Flutter client until the native app reaches parity.

## Brand Commitments

The app is Mosaic Fantasy. Preserve the website's dark palette and exact per-sport colors. Sport identity appears as a labelled, softly glowing dot rather than a colored card edge. The interface should feel native to iPhone, with no repeated global Mosaic Fantasy header.

## Evidence on Hand

The website's `frontend/src/app.css` contains the color tokens. `mobile/lib` contains the current account, Leagues, and League flows and JSON contracts. The Go backend is the source of truth for account and gameplay behavior.

## Product Principles

- Show only a manager's playable league data, never another dynasty by accident.
- Make first-run, empty, loading, and error states understandable.
- Keep frequent league tasks easy to scan and navigate on an iPhone.
- Prefer native interaction patterns over a web-shaped interface.

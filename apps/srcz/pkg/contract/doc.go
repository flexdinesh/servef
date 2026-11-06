// Package contract is the single source of truth for srcz's transport
// shapes (decision 0080): the JSON bodies every transport carries, the error
// body, and the request parameters clients send. internal/app returns these
// types; internal/web encodes them; pkg/client decodes them.
//
// tools/contracts generates packages/contracts (TypeScript types and runtime
// guards) from the files it lists as generated; the rest stay Go-only until
// their TypeScript migrates. Type docs may declare cross-field rules the
// guards enforce, one per line:
//
//	//contract:rule <field> ==|!= <JSON literal> => <field> ==|!= <JSON literal>
//
// Field names are JSON names. Keep field order: it is the wire order.
package contract

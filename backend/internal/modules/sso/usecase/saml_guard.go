package usecase

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"
)

const samlAssertionNS = "urn:oasis:names:tc:SAML:2.0:assertion"

var errSAMLShape = errors.New("saml response shape rejected")

// checkSAMLResponse rejects the response shapes behind GO-2026-6909 before
// crewjam/saml parses them; the library has no fixed release yet.
//
//  1. An assertion without <saml:Issuer> makes validateAssertion dereference nil.
//  2. Subject.NameID is matched by local name only, so a <NameID> in a foreign
//     namespace is read as the subject.
//
// Encrypted assertions cannot be inspected here and this service provider has
// no decryption key, so they are refused as well.
func checkSAMLResponse(r *http.Request) error {
	raw, e := base64.StdEncoding.DecodeString(r.PostForm.Get("SAMLResponse"))
	if e != nil || len(raw) == 0 || len(raw) > 1<<20 {
		return errSAMLShape
	}
	doc := etree.NewDocument()
	if doc.ReadFromBytes(raw) != nil || doc.Root() == nil {
		return errSAMLShape
	}
	assertions := 0
	var walk func(el *etree.Element) error
	walk = func(el *etree.Element) error {
		if el.Tag == "EncryptedAssertion" {
			return errSAMLShape
		}
		if el.Tag == "Assertion" {
			if el.NamespaceURI() != samlAssertionNS {
				return errSAMLShape
			}
			assertions++
			issuer := false
			for _, child := range el.ChildElements() {
				switch child.Tag {
				case "Issuer":
					if child.NamespaceURI() != samlAssertionNS {
						return errSAMLShape
					}
					issuer = true
				case "Subject":
					if child.NamespaceURI() != samlAssertionNS {
						return errSAMLShape
					}
					for _, sub := range child.ChildElements() {
						if sub.Tag == "NameID" && sub.NamespaceURI() != samlAssertionNS {
							return errSAMLShape
						}
					}
				}
			}
			if !issuer {
				return errSAMLShape
			}
		}
		for _, child := range el.ChildElements() {
			if e := walk(child); e != nil {
				return e
			}
		}
		return nil
	}
	if e := walk(doc.Root()); e != nil {
		return e
	}
	if assertions == 0 {
		return errSAMLShape
	}
	return nil
}

// parseSAMLResponse runs the shape check and turns a library panic into a
// denied login instead of a crashed request.
func parseSAMLResponse(sp *saml.ServiceProvider, r *http.Request, requestIDs []string) (assertion *saml.Assertion, err error) {
	if err = checkSAMLResponse(r); err != nil {
		return nil, err
	}
	defer func() {
		if recover() != nil {
			assertion, err = nil, errSAMLShape
		}
	}()
	return sp.ParseResponse(r, requestIDs)
}

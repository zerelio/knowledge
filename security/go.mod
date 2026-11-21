module github.com/zerelio/zerelio/security

go 1.26.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/zerelio/zerelio v0.1.0
)

// Local development: use the sibling platform module directly.
replace github.com/zerelio/zerelio => ../

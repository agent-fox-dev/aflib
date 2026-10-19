module example.com/m

go 1.27

require github.com/google/uuid v1.6.0

require (
	example.com/bad v1.0.0 // indirect
	example.com/worse v2.0.0+incompatible // indirect
)

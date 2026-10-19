module example.com/m // the module

go 1.27

toolchain go1.27.2

require example.com/single v1.0.0

require (
	example.com/direct v1.1.0
	example.com/quiet v1.2.0 // indirect
	// example.com/commented v0.0.0
	example.com/tail v1.3.0 // some other comment
)

replace example.com/a => ../a

replace (
	example.com/b => ../b
	example.com/c v1.0.0 => example.com/d v1.0.1
)

exclude example.com/e v1.0.0

require	(
	example.com/tabbed	v1.4.0
)

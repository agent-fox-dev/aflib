module example.com/m

go 1.27

replace (
	example.com/x => ../x
	example.com/y v1.2.3 => example.com/z v1.0.0
)

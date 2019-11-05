package graphql

func PathErrorInit(inner error, path []string) error {
	return &pathError{
		inner: inner,
		path:  path,
	}
}

type PathError = pathError

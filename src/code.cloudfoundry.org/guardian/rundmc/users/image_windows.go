package users

import "fmt"

func LookupImageUser(root, name string) (*ExecUser, error) {
	return nil, fmt.Errorf("image-backed runsc pea user lookup requires Linux")
}

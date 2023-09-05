// Copyright(C) 2023 github.com/fsgo  All Rights Reserved.
// Author: hidu <duv123@gmail.com>
// Date: 2023/9/5

package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerConf_getDeployTo(t *testing.T) {
	t.Run("case 1", func(t *testing.T) {
		cf1 := &ServerConf{
			Deploy: []*ServerConfDeploy{
				{
					From: "/search/",
					To:   "/home/work/app1/",
				},
				{
					From: "/search/",
					To:   "/home/work/app2/",
				},
				{
					From: "search/",
					To:   "/home/work/app3/",
				},
			},
		}
		got1 := cf1.getDeployTo("search/index.html")
		want1 := []string{
			"/home/work/app1/index.html",
			"/home/work/app2/index.html",
			"/home/work/app3/index.html",
		}
		require.Equal(t, want1, got1)
	})
}

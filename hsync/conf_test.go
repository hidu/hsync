package hsync

import (
	"testing"	
)

func TestConfRegexp(t *testing.T){
	casesMatch:=map[string][]string{
		"/*":[]string{"a","/a","/a/a","a/www"},
		"/www/*":[]string{"/www/a","/www/a.c","/www/a/b/c.c"},
		"/www/*.c":[]string{"/www/a.c","/www/a.a.c","/www/a./a.c"},
		"/www/*/*.c":[]string{"/www/a/a.c"},
		"*.c":[]string{"a.c","a.a.c",".c","a/a/a.c"},
		"d/*.c":[]string{"/d/a.c","ad/c.c","w/b/d/a.c"},
	}
	
	for r,exps:=range casesMatch{
		cr,err:=NewCongRegexp([]string{r})
		if(err!=nil){
			t.Error("init ConfRegexp failed",err)
			continue
		}
		for _,e:=range exps{
			if(!cr.IsMatch(e)){
				t.Error("cr [",r,"] should mutch [",e,"]")
			}
		}
	}
}
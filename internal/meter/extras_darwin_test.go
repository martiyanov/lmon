//go:build darwin

package meter

import "testing"

func TestParseNetstatIB(t *testing.T) {
	out := `Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll
lo0        16384 <Link#1>                      100     0       2048       100     0       2048     0
en0        1500  <Link#14>    aa:bb:cc:dd:ee:ff 1000    1    1234567      900     2    7654321     0
en0        1500  macbook-pro  fe80::1           1000    -    1234567      900     -    7654321     -
utun3      1500  <Link#20>                      10      0        100        10     0        100     0
en1        1500  <Link#15>    aa:bb:cc:dd:ee:00 20      0        300        30     0        400     0
`
	rx, tx, errs := parseNetstatIB(out)
	if rx != 1234867 || tx != 7654721 || errs != 3 {
		t.Fatalf("got rx=%d tx=%d errs=%d", rx, tx, errs)
	}
}

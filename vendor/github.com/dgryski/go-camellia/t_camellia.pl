#!/usr/bin/perl

# to run the full verification suite:
# curl http://info.isl.ntt.co.jp/crypt/eng/camellia/dl/cryptrec/t_camellia.txt |perl ./t_camellia.pl >cfull_test.go
# perl -pi -e 's/range camelliaTests/$&Full/' camellia_test.go
# go test

print <<GOCODE;
package camellia

var camelliaTestsFull = []struct {
        key    []byte
        plain  []byte
        cipher []byte
}{
GOCODE

my $k;
my $p;
my $c;

# K No.010 : EF CD AB 89 67 45 23 01 10 32 54 76 98 BA DC FE 10 32 54 76 98 BA DC FE 
sub linetostr {
    my $l = shift;

    $l =~ s/.*: *//;
    $l =~ s/\s*$//;
    my @h = split / /, $l;
    $l = join(",", map "0x$_", @h);
    return $l;
}


while(<>) {
    next if /^\s*$/ or /^Camellia/;
    if (/^K/) {
        $k = linetostr($_);
    }

    if (/^P/) {
        $p = linetostr($_);
    }

    if (/^C/) {
        $c = linetostr($_);
        print <<"GOCODE";
            {
                []byte{$k},
                []byte{$p},
                []byte{$c},
            },
GOCODE
    }
}

print <<GOCODE
}
GOCODE

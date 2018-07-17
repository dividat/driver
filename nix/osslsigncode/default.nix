{stdenv, fetchurl, openssl, curl, autoconf}:
stdenv.mkDerivation rec {
  name = "osslsigncode-${version}";
  version = "1.7.1";

  buildInputs = [ openssl curl autoconf ];

  src = fetchurl {
    url = "https://downloads.sourceforge.net/project/osslsigncode/osslsigncode/osslsigncode-${version}.tar.gz";
    sha256 = "f9a8cdb38b9c309326764ebc937cba1523a3a751a7ab05df3ecc99d18ae466c9";
  };
}

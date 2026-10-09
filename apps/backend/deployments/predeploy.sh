#!/bin/sh
set -eu
monacoctl migrate apply
monacoctl bus apply

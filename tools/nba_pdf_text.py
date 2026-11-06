#!/usr/bin/env python3
"""Linux PDF text adapter; requires the poppler-utils pdftotext executable."""
import subprocess
import sys

for path in sys.argv[1:]:
    subprocess.run(['pdftotext', '-layout', path, path+'.txt'], check=True)

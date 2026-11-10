## Design before features

A feature must fit the architecture: in the layer that owns it, through the
existing seams (the tree, confcheck, the builder, render, the agent), keeping the
invariants below. If it can't be added that way without a workaround (a special
case in a layer that shouldn't know about it, duplicated logic, state kept beside
the tree), don't build the workaround. Stop, before writing code or as soon as it
shows up, and tell the maintainer: what doesn't fit and why, the refactor you'd
recommend and its rough size, and any smaller part of the feature that fits
cleanly now. Go on only once they've chosen.

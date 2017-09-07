## Tips:

* commit message titles must be in the form:
```topic: Capitalized message with no trailing period```
or:
```topic, topic2: Capitalized message with no trailing period```

* please rebase your patch against current git master:
```
git checkout master
git pull origin master
git checkout your-feature
git rebase master
git push your-remote your-feature
hub pull-request	# or submit with the github web ui
```

* after a patch review, please ping @purpleidea so we know to re-review:
```
# make changes based on reviews...
git add -p		# add new changes
git commit --amend	# combine with existing commit
git push your-remote your-feature -f
# now ping @purpleidea in the github PR since it doesn't notify us automatically
```

## Thanks for contributing to mgmt and welcome to the team!

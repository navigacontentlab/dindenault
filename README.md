# dindenault

## Implementation example

    import "github.com/navigacontentlab/dindenault"

    app := dindenault.New(logger,
		dindenault.WithService(rpcconnect.FirstServiceHandler(firstService)),
		dindenault.WithService(rpcconnect.SecondServiceHandler(secondService)),
		dindenault.WithMiddleware(
			dindenault.WithLogging(logger),
			dindenault.WithXRay("projectname"),
		),
	)

	lambda.Start(app.Handle())
